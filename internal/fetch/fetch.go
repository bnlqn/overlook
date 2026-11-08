// Package fetch is how adapters read their sources: each through a Source,
// with GET requests under its address only, a login where a source offers no
// other way in, and NUT's LIST VAR, and nothing else, so that no key Overlook
// holds can change anything, or go anywhere but its own source, whatever an
// adapter asks for (ADR 0001, ADR 0009, ADR 0013).
package fetch

import (
	"cmp"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
)

// maxAnswer is the largest answer read from a source. The largest known,
// Radarr's whole library, is about a megabyte.
const maxAnswer = 32 << 20

// guard refuses every request but a GET, and a POST to login, the one URL a
// Source logs in at, if it has one (ADR 0009). A redirect is a request of
// its own, so a login is never sent on to another URL, nor sent twice.
type guard struct {
	next  http.RoundTripper
	login string
}

func (g guard) RoundTrip(r *http.Request) (*http.Response, error) {
	login := r.Method == http.MethodPost && g.login != "" && r.URL.String() == g.login && r.Response == nil
	if r.Method != http.MethodGet && !login {
		if r.Body != nil {
			_ = r.Body.Close()
		}
		return nil, fmt.Errorf("overlook only reads: %s refused", r.Method)
	}
	return g.next.RoundTrip(r)
}

// sameOrigin follows a redirect only to the scheme, host and port the read
// began at, such as /api to /api/. Go sends a redirect on with every header
// but Authorization and cookies, so a key in another header, such as
// X-Api-Key, would otherwise go wherever a source sends Overlook, such as an
// identity provider's host in front of it. It keeps Go's limit of 10.
func sameOrigin(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	if from := via[0].URL; req.URL.Scheme != from.Scheme || !strings.EqualFold(req.URL.Host, from.Host) {
		return errors.New("a redirect to another scheme, host or port refused")
	}
	return nil
}

// do sends req with hc and reads the answer, which must be a 200, giving back
// its header and body. Its error is short enough for the page.
func do(hc *http.Client, req *http.Request) (http.Header, []byte, error) {
	req.Header.Set("User-Agent", "Overlook")
	res, err := hc.Do(req) //nolint:gosec // the owner names their sources, and the guard decides what reaches them
	if err != nil {
		var u *url.Error
		if errors.As(err, &u) {
			if u.Timeout() {
				return nil, nil, errTimeout
			}
			return nil, nil, u.Err
		}
		return nil, nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, nil, status(res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, maxAnswer+1))
	if err != nil {
		return nil, nil, fmt.Errorf("reading the answer: %w", err)
	}
	if len(body) > maxAnswer {
		return nil, nil, fmt.Errorf("an answer over %d MiB", maxAnswer>>20)
	}
	return res.Header, body, nil
}

// errTimeout is a source that didn't answer in time.
var errTimeout = errors.New("timeout")

// Why says what kind of failure err is, so that the setup page can say what
// to do about it (ADR 0014): certificate, for one signed by a CA the system
// doesn't trust; certificate_name, for one for another name; plain, for an
// http:// source read as https://; refused, no_host or timeout; key, for a
// key or login refused; not_found; or nothing, for any other.
func Why(err error) string {
	var (
		authority x509.UnknownAuthorityError
		name      x509.HostnameError
		record    tls.RecordHeaderError
		dns       *net.DNSError
		code      status
	)
	switch {
	case errors.As(err, &authority):
		return "certificate"
	case errors.As(err, &name):
		return "certificate_name"
	// net/http says it of an answer in HTTP only in words; anything else
	// that isn't TLS fails its first record.
	case errors.As(err, &record), err != nil && strings.Contains(err.Error(), "server gave HTTP response to HTTPS client"):
		return "plain"
	case errors.Is(err, syscall.ECONNREFUSED):
		return "refused"
	case errors.As(err, &dns):
		return "no_host"
	case errors.Is(err, errTimeout):
		return "timeout"
	case errors.As(err, &code) && (code == http.StatusUnauthorized || code == http.StatusForbidden):
		return "key"
	case errors.As(err, &code) && code == http.StatusNotFound:
		return "not_found"
	}
	return ""
}

// status is a source answering with a status other than 200.
type status int

func (s status) Error() string { return fmt.Sprintf("HTTP %d", int(s)) }

// Basic is the header that sends a source its user and password, by HTTP's
// basic authentication, or none without a user. Unlike a login in a URL, it
// takes any password as it is.
func Basic(user, password string) http.Header {
	if user == "" {
		return nil
	}
	return http.Header{"Authorization": {"Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+password))}}
}

// CheckURL refuses a URL Overlook couldn't read or link to: none, one that
// isn't http or https, or one with no host. It refuses a login in one too,
// which a password's own characters can break and a link would show on the
// page. Its error follows the URL's name, as "url is missing".
func CheckURL(rawURL string) error {
	if rawURL == "" {
		return errors.New("is missing")
	}
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

// CheckLink fills a source's link, where the page sends the owner, with its
// url when the config gives none, and refuses one CheckURL would.
func CheckLink(link *string, sourceURL string) error {
	*link = cmp.Or(*link, sourceURL)
	if err := CheckURL(*link); err != nil {
		return fmt.Errorf("link %w", err)
	}
	return nil
}
