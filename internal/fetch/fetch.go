// Package fetch is how adapters read their sources: with GET requests, and
// NUT's read commands, and nothing else, so that no key Overlook holds can
// change anything, whatever an adapter asks for (ADR 0001).
package fetch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

// JSON reads the answer at rawURL into v. Its error is short enough for the
// page and never holds the URL, which may hold a key.
func JSON(ctx context.Context, rawURL string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return errors.New("bad url")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Overlook")
	res, err := client.Do(req)
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

// CheckURL refuses a URL Overlook couldn't read or link to: one that isn't
// http or https, or has no host.
func CheckURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return errors.New("isn't a URL")
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("needs to be an http:// or https:// URL")
	}
	return nil
}
