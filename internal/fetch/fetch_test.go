package fetch

import (
	"context"
	"encoding/pem"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// open readies s, failing t if it can't.
func open(t *testing.T, s Source) Source {
	t.Helper()
	if err := s.Check(); err != nil {
		t.Fatal(err)
	}
	return s
}

// caFile writes the certificate srv answers with to a PEM file, as the CA
// that signed a source's own, and returns its path.
func caFile(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	return ca
}

// Whichever way Check builds a source's client, plain, trusting a CA or
// logging in, it sends nothing but a GET, and its login's one POST.
func TestGuard(t *testing.T) {
	t.Parallel()
	var reached atomic.Int32
	count := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached.Add(1) })
	plain, signed := httptest.NewServer(count), httptest.NewTLSServer(count)
	t.Cleanup(plain.Close)
	t.Cleanup(signed.Close)
	login := &Login{Path: "/api/auth", Credentials: map[string]string{"password": "s3cret"}, Take: sid}

	for name, s := range map[string]Source{
		"plain":    open(t, Source{URL: plain.URL}),
		"trusting": open(t, Source{URL: signed.URL, CA: caFile(t, signed)}),
		"login":    open(t, Source{URL: plain.URL, Login: login}),
	} {
		for _, r := range []struct{ method, path string }{
			{"POST", "/"},
			{"POST", "/api/auth?again=1"},
			{"PUT", "/api/auth"},
			{"PATCH", "/"},
			{"DELETE", "/"},
			{"HEAD", "/"},
			{"OPTIONS", "/"},
		} {
			req, err := http.NewRequestWithContext(t.Context(), r.method, s.URL+r.path, nil)
			if err != nil {
				t.Fatal(err)
			}
			if res, err := s.client.Do(req); err == nil {
				_ = res.Body.Close()
				t.Errorf("%s: %s %s went through", name, r.method, r.path)
			}
		}
	}
	if n := reached.Load(); n != 0 {
		t.Errorf("%d requests reached the source", n)
	}
}

func TestJSON(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			_, _ = w.Write([]byte(`{"n": 4}`))
		case "/big":
			_, _ = w.Write([]byte(`"` + strings.Repeat("x", maxAnswer) + `"`))
		case "/html":
			_, _ = w.Write([]byte(`<html>`))
		default:
			http.Error(w, "no", http.StatusUnauthorized)
		}
	}))
	t.Cleanup(srv.Close)
	s := open(t, Source{URL: srv.URL})

	var v struct{ N int }
	if err := s.JSON(context.Background(), "/ok", &v); err != nil || v.N != 4 {
		t.Errorf("read %+v, %v", v, err)
	}
	for path, want := range map[string]string{"/key": "HTTP 401", "/big": "over 32 MiB", "/html": "isn't the JSON"} {
		err := s.JSON(context.Background(), path, &v)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: want an error with %q, got %v", path, want, err)
		}
	}
	if err := (Source{URL: srv.URL}).JSON(context.Background(), "/ok", &v); err == nil || !strings.Contains(err.Error(), "before its Check") {
		t.Errorf("a source read before its Check: %v", err)
	}

	gone := httptest.NewServer(nil)
	gone.Close()
	keyed := open(t, Source{URL: gone.URL, Header: http.Header{"X-Api-Key": {"s3cret"}}})
	err := keyed.JSON(context.Background(), "/x", &v)
	if err == nil || strings.Contains(err.Error(), "s3cret") || strings.Contains(err.Error(), "/x") {
		t.Errorf("an error mustn't hold the URL or the key: %v", err)
	}
}

// A path is read under the source's address, its own path kept, and one
// that doesn't begin with / is refused before anything is sent, so that no
// read, and no key, reaches another host.
// One item of a list that can't be read is left out, and the rest read,
// wherever the list is; anything else that can't be read is an error.
func TestDecode(t *testing.T) {
	t.Parallel()
	type check struct {
		Name    string    `json:"name"`
		At      time.Time `json:"at"`
		Results []struct {
			At time.Time `json:"at"`
		} `json:"results"`
	}
	var v struct {
		Version int              `json:"version"`
		Checks  []check          `json:"checks"`
		Groups  map[string][]int `json:"groups"`
	}
	odd := `{
		"version": 3,
		"checks": [
			{"name": "dns", "at": "2026-03-16T16:24:44Z"},
			{"name": "photos", "at": "2026-03-16 16:24:44"},
			{"name": "films", "at": "2026-03-16T16:24:44Z", "results": [{"at": "2026-03-16T16:24:44Z"}, {"at": 1773678284}]}
		],
		"groups": {"apps": [1, "two", 3]}
	}`
	if err := Decode([]byte(odd), &v); err != nil {
		t.Fatal(err)
	}
	if v.Version != 3 || len(v.Checks) != 2 || v.Checks[0].Name != "dns" || v.Checks[1].Name != "films" || len(v.Checks[1].Results) != 1 {
		t.Errorf("the photos check, and one of the films check's results, left out: %+v", v)
	}
	if !slices.Equal(v.Groups["apps"], []int{1, 3}) {
		t.Errorf("an item of a list in a map left out: %v", v.Groups)
	}

	// What isn't in a list fails the whole answer.
	for _, bad := range []string{`{"version": "three"}`, `{"checks": {"name": "dns"}}`, `[`} {
		if err := Decode([]byte(bad), &v); err == nil {
			t.Errorf("%s read as %+v", bad, v)
		}
	}
	// An answer that is a list is mended like any other.
	var list []check
	if err := Decode([]byte(`[{"at": "noon"}, {"name": "dns"}]`), &list); err != nil || len(list) != 1 || list[0].Name != "dns" {
		t.Errorf("read %+v, %v", list, err)
	}
}

func TestPath(t *testing.T) {
	t.Parallel()
	var reached []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = append(reached, r.URL.RequestURI())
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	var elsewhere atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { elsewhere.Add(1) }))
	t.Cleanup(other.Close)

	s := open(t, Source{URL: srv.URL + "/pihole/", Header: http.Header{"X-Api-Key": {"s3cret"}}})
	for _, path := range []string{"/api/stats?blocked=true", ""} {
		if err := s.JSON(t.Context(), path, new(struct{})); err != nil {
			t.Errorf("%q: %v", path, err)
		}
	}
	if want := []string{"/pihole/api/stats?blocked=true", "/pihole/"}; !slices.Equal(reached, want) {
		t.Errorf("read %q, want %q", reached, want)
	}

	bare := open(t, Source{URL: srv.URL, Header: http.Header{"X-Api-Key": {"s3cret"}}})
	for _, path := range []string{"api/stats", "@" + strings.TrimPrefix(other.URL, "http://") + "/", "?q=1"} {
		if err := bare.JSON(t.Context(), path, new(struct{})); err == nil || !strings.Contains(err.Error(), "begin with /") {
			t.Errorf("%q: want it refused, got %v", path, err)
		}
	}
	if len(reached) != 2 || elsewhere.Load() != 0 {
		t.Errorf("a refused path reached a server: %q, %d elsewhere", reached, elsewhere.Load())
	}
}

// A read asks for what Header's Accept names, such as a version of an API
// or an iCalendar, and otherwise for JSON or anything; the key goes too.
func TestAccept(t *testing.T) {
	t.Parallel()
	var accept, key string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accept, key = r.Header.Get("Accept"), r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	token := http.Header{"Authorization": {"Token 0123abcd"}}
	versioned := http.Header{"Authorization": {"Token 0123abcd"}, "Accept": {"application/json; version=10"}}

	for _, c := range []struct {
		header http.Header
		text   bool
		want   string
	}{
		{token, false, "application/json"},
		{token, true, "*/*"},
		{versioned, false, "application/json; version=10"},
		{http.Header{"Accept": {"text/calendar"}, "Authorization": {"Token 0123abcd"}}, true, "text/calendar"},
	} {
		s := open(t, Source{URL: srv.URL, Header: c.header})
		var err error
		if c.text {
			_, err = s.Text(t.Context(), "")
		} else {
			err = s.JSON(t.Context(), "/", new(struct{}))
		}
		if err != nil || accept != c.want || key != "Token 0123abcd" {
			t.Errorf("asked for %q with %q, want %q with the key: %v", accept, key, c.want, err)
		}
	}
}

// A redirect within the source is followed, and one to another host is
// refused before it goes, so a key in a header Go sends on never reaches it.
func TestRedirect(t *testing.T) {
	t.Parallel()
	var reached atomic.Int32
	elsewhere := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached.Add(1) }))
	t.Cleanup(elsewhere.Close)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api":
			http.Redirect(w, r, "/api/", http.StatusMovedPermanently)
		case "/api/":
			_, _ = w.Write([]byte(`{"n": 4}`))
		default:
			http.Redirect(w, r, elsewhere.URL+"/login", http.StatusFound)
		}
	}))
	t.Cleanup(srv.Close)
	s := open(t, Source{URL: srv.URL, Header: http.Header{"X-Api-Key": {"s3cret"}}})

	var v struct{ N int }
	if err := s.JSON(t.Context(), "/api", &v); err != nil || v.N != 4 {
		t.Errorf("read %+v, %v", v, err)
	}
	err := s.JSON(t.Context(), "/library", &v)
	if err == nil || !strings.Contains(err.Error(), "redirect") {
		t.Errorf("want a redirect refused, got %v", err)
	}
	if err != nil && (strings.Contains(err.Error(), "127.0.0.1") || strings.Contains(err.Error(), "/login")) {
		t.Errorf("an error mustn't hold the URL: %v", err)
	}
	if n := reached.Load(); n != 0 {
		t.Errorf("%d requests reached the other host", n)
	}
}

// A source that signs its own certificate is read with its CA, and with
// nothing else.
func TestTrusting(t *testing.T) {
	t.Parallel()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"n": 4}`))
	}))
	t.Cleanup(srv.Close)
	ca := caFile(t, srv)

	var v struct{ N int }
	if err := open(t, Source{URL: srv.URL}).JSON(context.Background(), "", &v); err == nil || !strings.Contains(err.Error(), "certificate") {
		t.Errorf("a certificate the system doesn't trust is refused: %v", err)
	}
	if err := open(t, Source{URL: srv.URL, CA: ca}).JSON(context.Background(), "", &v); err != nil || v.N != 4 {
		t.Errorf("read %+v, %v", v, err)
	}
	// The test certificate is for example.com, *.example.com and 127.0.0.1.
	for name, ok := range map[string]bool{"example.com": true, "backup.example.org": false} {
		named := open(t, Source{URL: srv.URL, CA: ca, ServerName: name})
		if err := named.JSON(context.Background(), "", &v); (err == nil) != ok {
			t.Errorf("a certificate checked for %s: %v", name, err)
		}
	}
}

// Check refuses a source it couldn't read, naming the config's field.
func TestCheck(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	notPEM := filepath.Join(dir, "ca.txt")
	if err := os.WriteFile(notPEM, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		s    Source
		want string
	}{
		{Source{}, "url is missing"},
		{Source{URL: "https://overlook:s3cret@pihole.example.com"}, "url can't hold a login"},
		{Source{URL: "https://pbs.example.com", ServerName: "pbs.example.com"}, "server_name needs the ca"},
		{Source{URL: "https://pbs.example.com", CA: notPEM}, "ca: " + notPEM + " holds no PEM certificate"},
		{Source{URL: "https://pbs.example.com", CA: filepath.Join(dir, "none.pem")}, "ca: open"},
		{Source{URL: "https://pihole.example.com", Login: &Login{Path: "api/auth"}}, "login's path needs to begin with /"},
		{Source{URL: "https://pihole.example.com", Login: &Login{}}, "login's path needs to begin with /"},
	} {
		if err := c.s.Check(); err == nil || !strings.HasPrefix(err.Error(), c.want) {
			t.Errorf("%+v: want an error beginning %q, got %v", c.s, c.want, err)
		}
	}
}

func TestCheckURL(t *testing.T) {
	t.Parallel()
	for raw, ok := range map[string]bool{
		"":                                false,
		"https://status.example.com":      true,
		"http://gatus:8080":               true,
		"javascript:alert(1)":             false,
		"status.example.com":              false,
		"ftp://files.example.com":         false,
		"https://":                        false,
		"https://user:pass@x.example.com": false,
	} {
		if err := CheckURL(raw); (err == nil) != ok {
			t.Errorf("%s: %v", raw, err)
		}
	}
}

// Each failure the setup page says what to do about is told apart.
func TestWhy(t *testing.T) {
	t.Parallel()
	refuse := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/key":
			http.Error(w, "", http.StatusUnauthorized)
		case "/slow":
			<-r.Context().Done()
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(refuse.Close)
	signed := httptest.NewTLSServer(http.NotFoundHandler())
	t.Cleanup(signed.Close)
	gone := httptest.NewServer(nil)
	gone.Close()
	// A server that answers a TLS handshake with neither TLS nor HTTP.
	junk, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { junk.Close() })
	go func() {
		for {
			c, err := junk.Accept()
			if err != nil {
				return
			}
			_, _ = c.Write([]byte("SSH-2.0-example\r\n"))
			c.Close()
		}
	}()
	told := map[string]bool{}
	why := func(what string, err error, want string) {
		t.Helper()
		told[want] = true
		if got := Why(err); got != want {
			t.Errorf("%s: %q, want %q, from %v", what, got, want, err)
		}
	}
	for _, tc := range []struct{ url, path, want string }{
		{signed.URL, "", "certificate"},
		{strings.Replace(refuse.URL, "http:", "https:", 1), "", "plain"},
		{"https://" + junk.Addr().String(), "", "plain"},
		{gone.URL, "", "refused"},
		{"http://overlook.invalid", "", "no_host"},
		{refuse.URL, "/key", "key"},
		{refuse.URL, "/none", "not_found"},
	} {
		// No deadline here: a busy machine can take a second over a TLS
		// handshake, and a deadline would turn its answer into a timeout.
		why(tc.url+tc.path, open(t, Source{URL: tc.url}).JSON(t.Context(), tc.path, new(any)), tc.want)
	}
	// A source that never answers is the one case that needs a deadline.
	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()
	why("never answers", open(t, Source{URL: refuse.URL}).JSON(ctx, "/slow", new(any)), "timeout")
	// A certificate for another name: the CA is trusted, the name isn't it.
	s := open(t, Source{URL: strings.Replace(signed.URL, "127.0.0.1", "localhost", 1), CA: caFile(t, signed)})
	why("another name", s.JSON(t.Context(), "", new(any)), "certificate_name")
	// Kinds names every kind told apart here, and only those.
	if kinds := slices.Sorted(maps.Keys(told)); !slices.Equal(kinds, slices.Sorted(slices.Values(Kinds))) {
		t.Errorf("told apart %q, but Kinds is %q", kinds, Kinds)
	}
}
