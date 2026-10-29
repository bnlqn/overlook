package adaptertest

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
)

func TestReplay(t *testing.T) {
	t.Parallel()
	dir := recorded(t, map[string]string{
		"api/plain.json":           `{"n": 1}`,
		"api/pick@x=1.json":        `"x"`,
		"api/pick@x=1&y=2.json":    `"x and y"`,
		"api/only@at=12%3A00.json": `"noon"`,
		"api/login.http":           "HTTP/1.1 200 OK\r\nSet-Cookie: SID=0123abcd\r\nContent-Length: 3\r\n\r\nOk.",
		"record.sh":                "not an answer",
	})
	src := Replay(t, dir, `"n": 1`, `"n": 2`, "Ok.", "Okay.")
	for _, r := range []struct{ method, path, want string }{
		{http.MethodGet, "/api/plain?any=thing", `{"n": 2}`},
		{http.MethodGet, "/api/pick?y=2&x=1&z=3", `"x and y"`},
		{http.MethodGet, "/api/pick?x=1", `"x"`},
		{http.MethodGet, "/api/only?at=12:00", `"noon"`},
		{http.MethodPost, "/api/login", "Okay."},
		{http.MethodGet, "/api/elsewhere", "404 page not found\n"},
	} {
		h, body := send(t, r.method, src.URL+r.path)
		if body != r.want {
			t.Errorf("%s %s: %q, want %q", r.method, r.path, body, r.want)
		}
		if r.method == http.MethodPost && h.Get("Set-Cookie") != "SID=0123abcd" {
			t.Errorf("a login's cookie, from its header: %q", h.Get("Set-Cookie"))
		}
	}
	sent := src.Sent()
	if len(sent) != 6 || sent[4].Method != http.MethodPost || sent[4].Body != "user=overlook" || sent[3].URL.Query().Get("at") != "12:00" {
		t.Errorf("every request, in order, with its body: %+v", sent)
	}
	if h, _ := send(t, http.MethodGet, src.URL+"/api/plain"); h.Get("Content-Type") != "application/json" {
		t.Errorf("an answer's type, by its name: %q", h.Get("Content-Type"))
	}

	// What no source Replay plays would be sent fails the test.
	refused := failures(func(tb *fake) {
		src := Replay(tb, dir)
		send(tb, http.MethodGet, src.URL+"/api/only?at=13:00")
		send(tb, http.MethodPut, src.URL+"/api/plain")
		send(tb, http.MethodPost, src.URL+"/api/login")
		send(tb, http.MethodPost, src.URL+"/api/login")
	})
	if len(refused) != 3 || !strings.Contains(refused[0], "no answer recorded for its query") ||
		!strings.Contains(refused[1], "only read") || !strings.Contains(refused[2], "second login") {
		t.Errorf("a query with no answer, a PUT and a second login refused: %q", refused)
	}
	if unread := failures(func(tb *fake) { Replay(tb, dir) }); len(unread) != 5 || !strings.Contains(unread[0], "api/login.http: an answer nothing read") {
		t.Errorf("every answer nothing read: %q", unread)
	}
	if edit := failures(func(tb *fake) { Replay(tb, dir, "nowhere", "here") }); len(edit) != 1 {
		t.Errorf("an edit that changes nothing: %q", edit)
	}
	bad := recorded(t, map[string]string{"api/only@at=12:00.json": `"noon"`})
	if name := failures(func(tb *fake) { Replay(tb, bad) }); len(name) != 1 || !strings.Contains(name[0], `refuse a ':'`) {
		t.Errorf("a name Go's modules refuse: %q", name)
	}
}

// recorded writes answers, by their names, into a directory of their own.
func recorded(t *testing.T, answers map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, answer := range answers {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(answer), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// send sends method to url, a POST with a body, and gives back the answer's
// header and body.
func send(tb testing.TB, method, url string) (http.Header, string) {
	tb.Helper()
	var body io.Reader
	if method == http.MethodPost {
		body = strings.NewReader("user=overlook")
	}
	req, err := http.NewRequestWithContext(context.Background(), method, url, body)
	if err != nil {
		tb.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		tb.Fatal(err)
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		tb.Fatal(err)
	}
	return res.Header, string(b)
}

// failures runs f with a testing.TB of its own, and its cleanups after it,
// and gives back each way f failed it.
func failures(f func(*fake)) []string {
	tb := &fake{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			for _, c := range slices.Backward(tb.cleanups) {
				c()
			}
		}()
		f(tb)
	}()
	<-done
	return tb.failed
}

// fake is a testing.TB that keeps what fails it, so that a test can check
// Replay fails the tests it should. Its other methods are never called.
type fake struct {
	testing.TB
	mu       sync.Mutex
	failed   []string
	cleanups []func()
}

func (f *fake) Helper()                   {}
func (f *fake) Cleanup(c func())          { f.cleanups = append(f.cleanups, c) }
func (f *fake) Fatal(a ...any)            { f.Error(a...); runtime.Goexit() }
func (f *fake) Fatalf(s string, a ...any) { f.Errorf(s, a...); runtime.Goexit() }
func (f *fake) Error(a ...any)            { f.Errorf("%s", fmt.Sprint(a...)) }

func (f *fake) Errorf(s string, a ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failed = append(f.failed, fmt.Sprintf(s, a...))
}

func (f *fake) Failed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.failed) > 0
}
