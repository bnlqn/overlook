// Package fetch is how adapters read their sources: with GET requests, and
// NUT's read commands, and nothing else, so that no key Overlook holds can
// change anything, whatever an adapter asks for (ADR 0001).
package fetch

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
)

// maxAnswer is the largest answer read from a source. The largest known,
// Radarr's whole library, is about a megabyte.
const maxAnswer = 32 << 20

// getOnly refuses every request but a GET.
type getOnly struct{ next http.RoundTripper }

func (g getOnly) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Method != http.MethodGet {
		return nil, fmt.Errorf("overlook only reads: %s refused", r.Method)
	}
	return g.next.RoundTrip(r)
}

var client = &http.Client{Transport: getOnly{http.DefaultTransport}}

// Client reads a source. Its zero value trusts the CAs the system trusts;
// Trusting makes one that trusts the source's own.
type Client struct{ http *http.Client }

// Trusting returns a client for a source that signs its own certificate,
// such as a Proxmox host: it trusts only the CAs in the PEM file at caFile.
// With a serverName, the certificate must be for that name rather than the
// URL's host, for a source read at an address its certificate doesn't
// name. There is no client that skips the check, so a key is never sent to
// whoever answers in the source's place.
func Trusting(caFile, serverName string) (Client, error) {
	pem, err := os.ReadFile(caFile) //nolint:gosec // the owner names their own CA
	if err != nil {
		return Client{}, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return Client{}, fmt.Errorf("%s holds no PEM certificate", caFile)
	}
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.TLSClientConfig = &tls.Config{RootCAs: pool, ServerName: serverName, MinVersion: tls.VersionTLS12}
	return Client{&http.Client{Transport: getOnly{t}}}, nil
}

// JSON reads the answer at rawURL into v, sending header with the request,
// such as a key. Its error is short enough for the page and never holds the
// URL or the header, which may hold a key.
func JSON(ctx context.Context, rawURL string, header http.Header, v any) error {
	return Client{}.JSON(ctx, rawURL, header, v)
}

// JSON reads the answer at rawURL into v as the package's JSON does, with c.
func (c Client) JSON(ctx context.Context, rawURL string, header http.Header, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return errors.New("bad url")
	}
	for k, vs := range header {
		req.Header[http.CanonicalHeaderKey(k)] = vs
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Overlook")
	hc := c.http
	if hc == nil {
		hc = client
	}
	res, err := hc.Do(req)
	if err != nil {
		var u *url.Error
		if errors.As(err, &u) {
			if u.Timeout() {
				return errors.New("timeout")
			}
			return u.Err
		}
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, maxAnswer+1))
	if err != nil {
		return fmt.Errorf("reading the answer: %w", err)
	}
	if len(body) > maxAnswer {
		return fmt.Errorf("an answer over %d MiB", maxAnswer>>20)
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("an answer that isn't the JSON expected: %w", err)
	}
	return nil
}

// Login is the header that sends a source its user and password, by HTTP's
// basic authentication, or none without a user. Unlike a login in a URL, it
// takes any password as it is.
func Login(user, password string) http.Header {
	if user == "" {
		return nil
	}
	return http.Header{"Authorization": {"Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+password))}}
}

// CheckURL refuses a URL Overlook couldn't read or link to: one that isn't
// http or https, or has no host. It refuses a login in one too, which a
// password's own characters can break and a link would show on the page.
func CheckURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return errors.New("isn't a URL")
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("needs to be an http:// or https:// URL")
	}
	if u.User != nil {
		return errors.New("can't hold a login: a source that needs one takes user and password")
	}
	return nil
}
