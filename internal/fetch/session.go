package fetch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"sync"
)

// Login is a source's one login, for a source that lets Overlook in only
// after one, such as Pi-hole (ADR 0009): a POST to Path, under the source's
// URL, such as /api/auth, carrying Credentials and nothing else, as a JSON
// object, such as {"password": "…"}, or as a form, such as
// username=…&password=…, as qBittorrent takes them. Take reads the session
// from the login's answer, its header and body, and returns the header that
// sends it with each GET, such as X-FTL-SID.
type Login struct {
	Path        string
	Credentials map[string]string
	Form        bool
	Take        func(http.Header, []byte) (http.Header, error)
}

// session keeps the session a login gives in memory (ADR 0004), and gives it
// to every GET until the source refuses it; only then does it log in again.
// So an Overlook read every 30 seconds logs in once, not every time, and
// doesn't fill the source's list of sessions.
type session struct {
	client *http.Client
	url    string
	login  Login

	mu sync.Mutex
	// sent sends the session with each GET; it is nil until a login.
	sent http.Header
}

// refused says whether err is a source refusing a session: a 401, or a 403,
// which is how qBittorrent refuses one.
func refused(err error) bool {
	var s status
	return errors.As(err, &s) && (s == http.StatusUnauthorized || s == http.StatusForbidden)
}

// header returns the header that sends the session, logging in if there is
// none yet, or if stale, the one the source refused, is still the one kept.
// Reads refused at the same time log in once: the first logs in, and the
// others send its session.
func (s *session) header(ctx context.Context, stale http.Header) (http.Header, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sent != nil && (stale == nil || !maps.EqualFunc(s.sent, stale, slices.Equal)) {
		return s.sent, nil
	}
	var err error
	s.sent, err = s.logIn(ctx)
	return s.sent, err
}

// logIn sends the login its one POST and returns the header that sends the
// session it gets back, or nil and why there is none.
func (s *session) logIn(ctx context.Context) (http.Header, error) {
	body, err := json.Marshal(s.login.Credentials)
	if err != nil {
		return nil, err
	}
	kind := "application/json"
	if s.login.Form {
		form := url.Values{}
		for k, v := range s.login.Credentials {
			form.Set(k, v)
		}
		body, kind = []byte(form.Encode()), "application/x-www-form-urlencoded"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("bad url")
	}
	req.Header.Set("Content-Type", kind)
	req.Header.Set("Accept", "application/json")
	header, answer, err := do(s.client, req)
	if err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}
	session, err := s.login.Take(header, answer)
	if err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}
	// A login that gives nothing to send would be sent again with every GET.
	if len(session) == 0 {
		return nil, errors.New("login: no session in the answer")
	}
	return session, nil
}
