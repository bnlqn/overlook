package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

// getOnly refuses every request but a GET, so that no key Overlook holds can
// change anything, whatever an adapter asks for (ADR 0001).
type getOnly struct{ next http.RoundTripper }

func (g getOnly) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Method != http.MethodGet {
		return nil, fmt.Errorf("overlook only reads: %s refused", r.Method)
	}
	return g.next.RoundTrip(r)
}

// client is the only HTTP client adapters get.
var client = &http.Client{Transport: getOnly{http.DefaultTransport}}

// getJSON reads a source's answer into v. Its error never holds the URL,
// which may hold a key, and reaches the page.
func getJSON(ctx context.Context, rawURL string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return errors.New("bad url")
	}
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
	return json.NewDecoder(res.Body).Decode(v)
}
