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

// Session reads a source that lets Overlook in only after a login, such as
// Pi-hole (ADR 0009). It sends its login URL one POST, carrying only the
// credentials, keeps the session the answer gives in memory (ADR 0004), and
// sends it with every GET until the source refuses it; only then does it log
// in again. So an Overlook read every 30 seconds logs in once, not every
// time, and doesn't fill the source's list of sessions.
type Session struct {
	client      Client
	login       string
	credentials map[string]string
	// form sends the credentials as a form, as qBittorrent takes them,
	// rather than as JSON.
	form bool
	take func(http.Header, []byte) (http.Header, error)

	mu sync.Mutex
	// header sends the session with each GET; it is nil until a login.
	header http.Header
}

// Session returns a session for a source whose login is a POST to login,
// such as https://pihole.example.com/api/auth, read with c. The POST carries
// credentials as a JSON object, such as {"password": "…"}, and nothing else.
// take reads the session from the login's answer, its header and body, and
// returns the header that sends it with each GET, such as X-FTL-SID.
func (c Client) Session(login string, credentials map[string]string, take func(http.Header, []byte) (http.Header, error)) (*Session, error) {
	if err := CheckURL(login); err != nil {
		return nil, fmt.Errorf("login %w", err)
	}
	u, _ := url.Parse(login)
	hc := c.http
	if hc == nil {
		hc = client
	}
	g := hc.Transport.(guard)
	g.login = u.String()
	// The session follows redirects as c does, so it goes no further than
	// c's keys would.
	sc := &http.Client{Transport: g, CheckRedirect: hc.CheckRedirect}
	return &Session{client: Client{sc}, login: g.login, credentials: credentials, take: take}, nil
}

// FormSession returns a session as Session does, whose login sends the
// credentials as a form, such as username=…&password=…, as qBittorrent's
// does.
func (c Client) FormSession(login string, credentials map[string]string, take func(http.Header, []byte) (http.Header, error)) (*Session, error) {
	s, err := c.Session(login, credentials, take)
	if s != nil {
		s.form = true
	}
	return s, err
}

// JSON reads the answer at rawURL into v as the package's JSON does, sending
// the session, and logs in first if there is none or the source refuses it.
func (s *Session) JSON(ctx context.Context, rawURL string, v any) error {
	header, err := s.session(ctx, nil)
	if err != nil {
		return err
	}
	body, err := s.client.get(ctx, rawURL, "application/json", header)
	if refused(err) {
		if header, err = s.session(ctx, header); err != nil {
			return err
		}
		body, err = s.client.get(ctx, rawURL, "application/json", header)
	}
	if err != nil {
		return err
	}
	return decode(body, v)
}

// refused says whether err is a source refusing a session: a 401, or a 403,
// which is how qBittorrent refuses one.
func refused(err error) bool {
	var s status
	return errors.As(err, &s) && (s == http.StatusUnauthorized || s == http.StatusForbidden)
}

// session returns the session to send, logging in if there is none yet, or
// if stale, the one the source refused, is still the one kept. Reads refused
// at the same time log in once: the first logs in, and the others send its
// session.
func (s *Session) session(ctx context.Context, stale http.Header) (http.Header, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.header != nil && (stale == nil || !maps.EqualFunc(s.header, stale, slices.Equal)) {
		return s.header, nil
	}
	var err error
	s.header, err = s.logIn(ctx)
	return s.header, err
}

// logIn sends the login its one POST and returns the header that sends the
// session it gets back, or nil and why there is none.
func (s *Session) logIn(ctx context.Context) (http.Header, error) {
	body, err := json.Marshal(s.credentials)
	if err != nil {
		return nil, err
	}
	kind := "application/json"
	if s.form {
		form := url.Values{}
		for k, v := range s.credentials {
			form.Set(k, v)
		}
		body, kind = []byte(form.Encode()), "application/x-www-form-urlencoded"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.login, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("bad url")
	}
	req.Header.Set("Content-Type", kind)
	req.Header.Set("Accept", "application/json")
	header, answer, err := s.client.do(req)
	if err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}
	session, err := s.take(header, answer)
	if err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}
	// A login that gives nothing to send would be sent again with every GET.
	if len(session) == 0 {
		return nil, errors.New("login: no session in the answer")
	}
	return session, nil
}
