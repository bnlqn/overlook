package fetch

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// source is a fake source that lets a client in only after a login, as
// Pi-hole does. It counts logins and anything else that reaches it, refuses
// every other method and path, and forgets its session when told to.
type source struct {
	*httptest.Server
	logins, redirects, others atomic.Int32

	mu  sync.Mutex
	sid string
}

func newSource(t *testing.T) *source {
	t.Helper()
	s := &source{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/auth":
			n := s.logins.Add(1)
			body, _ := io.ReadAll(r.Body)
			if r.Header.Get("Content-Type") != "application/json" || string(body) != `{"password":"s3cret"}` {
				http.Error(w, `{"session": {"valid": false}}`, http.StatusUnauthorized)
				return
			}
			s.mu.Lock()
			s.sid = fmt.Sprintf("sid-%d", n)
			_, _ = fmt.Fprintf(w, `{"session": {"valid": true, "sid": %q}}`, s.sid)
			s.mu.Unlock()
		case r.Method == http.MethodPost && r.URL.Path == "/api/again":
			s.redirects.Add(1)
			http.Redirect(w, r, "/api/again", http.StatusTemporaryRedirect)
		case r.Method == http.MethodPost && r.URL.Path == "/api/away":
			s.redirects.Add(1)
			http.Redirect(w, r, "/api/auth", http.StatusPermanentRedirect)
		case r.Method == http.MethodGet && r.URL.Path == "/api/moved":
			http.Redirect(w, r, "/api/stats", http.StatusFound)
		case r.Method == http.MethodGet && r.URL.Path == "/api/stats":
			s.mu.Lock()
			in := s.sid != "" && r.Header.Get("X-FTL-SID") == s.sid
			s.mu.Unlock()
			if !in {
				http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"n": 4}`))
		default:
			s.others.Add(1)
			http.Error(w, "no", http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

// forget drops the source's session, as Pi-hole does after 30 idle minutes.
func (s *source) forget() {
	s.mu.Lock()
	s.sid = ""
	s.mu.Unlock()
}

// sid takes the session from a login's answer, as Pi-hole gives it.
func sid(_ http.Header, body []byte) (http.Header, error) {
	var a struct{ Session struct{ SID string } }
	if err := json.Unmarshal(body, &a); err != nil || a.Session.SID == "" {
		return nil, errors.New("no sid")
	}
	return http.Header{"X-FTL-SID": {a.Session.SID}}, nil
}

// readAll reads the source's stats n times at once, as an adapter reads its
// paths, and fails t on any error.
func readAll(t *testing.T, s *Session, url string, n int) {
	t.Helper()
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			var v struct{ N int }
			if err := s.JSON(t.Context(), url, &v); err != nil || v.N != 4 {
				t.Errorf("read %+v, %v", v, err)
			}
		})
	}
	wg.Wait()
}

// Reads log in once, keep the session, and log in once more when the source
// forgets it, however many read at the same time.
func TestSession(t *testing.T) {
	t.Parallel()
	src := newSource(t)
	s, err := Client{}.Session(src.URL+"/api/auth", map[string]string{"password": "s3cret"}, sid)
	if err != nil {
		t.Fatal(err)
	}
	stats := src.URL + "/api/stats"

	readAll(t, s, stats, 8)
	readAll(t, s, stats, 8)
	if n := src.logins.Load(); n != 1 {
		t.Errorf("16 reads logged in %d times, want once", n)
	}
	src.forget()
	readAll(t, s, stats, 8)
	if n := src.logins.Load(); n != 2 {
		t.Errorf("reads after the session was forgotten logged in %d times in all, want twice", n)
	}
	if n := src.others.Load(); n != 0 {
		t.Errorf("%d other requests reached the source", n)
	}
}

// A form session sends its login as a form, as qBittorrent takes it, the
// password's own characters escaped.
func TestFormSession(t *testing.T) {
	t.Parallel()
	var login string
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			login = r.Header.Get("Content-Type") + " " + string(body)
			_, _ = w.Write([]byte("Ok."))
			return
		}
		_, _ = w.Write([]byte(`{"n": 4}`))
	}))
	t.Cleanup(src.Close)
	s, err := Client{}.FormSession(src.URL+"/login", map[string]string{"username": "overlook", "password": "s3 cret&"},
		func(http.Header, []byte) (http.Header, error) { return http.Header{"Cookie": {"SID=1"}}, nil })
	if err != nil {
		t.Fatal(err)
	}
	readAll(t, s, src.URL+"/stats", 1)
	if want := "application/x-www-form-urlencoded password=s3+cret%26&username=overlook"; login != want {
		t.Errorf("the login sent %q, want %q", login, want)
	}
}

// A refused login is tried once per read, and its error holds neither the
// password nor the URL.
func TestSessionRefused(t *testing.T) {
	t.Parallel()
	src := newSource(t)
	var v struct{ N int }
	for _, c := range []struct {
		password string
		take     func(http.Header, []byte) (http.Header, error)
		want     string
	}{
		{"wrong", sid, "login: HTTP 401"},
		{"s3cret", func(http.Header, []byte) (http.Header, error) { return nil, nil }, "login: no session"},
	} {
		s, err := Client{}.Session(src.URL+"/api/auth", map[string]string{"password": c.password}, c.take)
		if err != nil {
			t.Fatal(err)
		}
		before := src.logins.Load()
		err = s.JSON(t.Context(), src.URL+"/api/stats", &v)
		if err == nil || !strings.Contains(err.Error(), c.want) || strings.Contains(err.Error(), c.password) || strings.Contains(err.Error(), "/api") {
			t.Errorf("want an error with %q and no password or URL, got %v", c.want, err)
		}
		if n := src.logins.Load() - before; n != 1 {
			t.Errorf("a read logged in %d times, want once", n)
		}
	}
}

// A session's client sends its login one POST, and refuses every other
// request but a GET: another method, a POST to another path, and a login
// sent on or sent again by a redirect.
func TestSessionGuard(t *testing.T) {
	t.Parallel()
	src := newSource(t)
	s, err := Client{}.Session(src.URL+"/api/auth", map[string]string{"password": "s3cret"}, sid)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range []struct{ method, path string }{
		{"POST", "/api/stats"},
		{"POST", "/api/auth?again=1"},
		{"PUT", "/api/auth"},
		{"PATCH", "/api/auth"},
		{"DELETE", "/api/auth"},
		{"HEAD", "/api/auth"},
		{"OPTIONS", "/api/auth"},
	} {
		req, err := http.NewRequestWithContext(t.Context(), r.method, src.URL+r.path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if res, err := s.client.http.Do(req); err == nil {
			_ = res.Body.Close()
			t.Errorf("%s %s went through", r.method, r.path)
		}
	}
	if n := src.logins.Load() + src.others.Load(); n != 0 {
		t.Errorf("%d refused requests reached the source", n)
	}

	for _, path := range []string{"/api/again", "/api/away"} {
		moved, err := Client{}.Session(src.URL+path, map[string]string{"password": "s3cret"}, sid)
		if err != nil {
			t.Fatal(err)
		}
		before := src.redirects.Load()
		if err := moved.JSON(t.Context(), src.URL+"/api/stats", new(struct{})); err == nil || !strings.Contains(err.Error(), "POST refused") {
			t.Errorf("%s: a login redirected went on: %v", path, err)
		}
		if n := src.redirects.Load() - before; n != 1 {
			t.Errorf("%s: the login was sent %d times, want once", path, n)
		}
	}
	if n := src.logins.Load(); n != 0 {
		t.Errorf("a redirected login reached the login %d times", n)
	}

	// A session follows redirects only as its client does.
	strict := Client{&http.Client{Transport: client.Transport, CheckRedirect: func(*http.Request, []*http.Request) error {
		return errors.New("redirect refused")
	}}}
	s, err = strict.Session(src.URL+"/api/auth", map[string]string{"password": "s3cret"}, sid)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.JSON(t.Context(), src.URL+"/api/moved", new(struct{})); err == nil || !strings.Contains(err.Error(), "redirect refused") {
		t.Errorf("a session followed a redirect its client refuses: %v", err)
	}

	if _, err := (Client{}).Session("https://overlook:s3cret@pihole.example.com/api/auth", nil, sid); err == nil {
		t.Error("a login URL holding a login was taken")
	}
}
