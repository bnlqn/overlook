package fetch

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestOnlyGET(t *testing.T) {
	t.Parallel()
	var reached atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached.Add(1) }))
	t.Cleanup(s.Close)
	for _, method := range []string{"POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"} {
		req, err := http.NewRequestWithContext(t.Context(), method, s.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		if res, err := client.Do(req); err == nil {
			_ = res.Body.Close()
			t.Errorf("%s went through", method)
		}
	}
	if n := reached.Load(); n != 0 {
		t.Errorf("%d requests reached the source", n)
	}
}

func TestJSON(t *testing.T) {
	t.Parallel()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	t.Cleanup(s.Close)

	var v struct{ N int }
	if err := JSON(context.Background(), s.URL+"/ok", nil, &v); err != nil || v.N != 4 {
		t.Errorf("read %+v, %v", v, err)
	}
	for path, want := range map[string]string{"/key": "HTTP 401", "/big": "over 32 MiB", "/html": "isn't the JSON"} {
		err := JSON(context.Background(), s.URL+path, nil, &v)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: want an error with %q, got %v", path, want, err)
		}
	}

	gone := httptest.NewServer(nil)
	gone.Close()
	err := JSON(context.Background(), "http://overlook:s3cret@"+strings.TrimPrefix(gone.URL, "http://")+"/x", nil, &v)
	if err == nil || strings.Contains(err.Error(), "s3cret") || strings.Contains(err.Error(), "/x") {
		t.Errorf("an error mustn't hold the URL: %v", err)
	}
}

// A redirect within the source is followed, and one to another host is
// refused before it goes, so a key in a header Go sends on never reaches it.
func TestRedirect(t *testing.T) {
	t.Parallel()
	var reached atomic.Int32
	elsewhere := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached.Add(1) }))
	t.Cleanup(elsewhere.Close)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api":
			http.Redirect(w, r, "/api/", http.StatusMovedPermanently)
		case "/api/":
			_, _ = w.Write([]byte(`{"n": 4}`))
		default:
			http.Redirect(w, r, elsewhere.URL+"/login", http.StatusFound)
		}
	}))
	t.Cleanup(s.Close)

	key := http.Header{"X-Api-Key": {"s3cret"}}
	var v struct{ N int }
	if err := JSON(t.Context(), s.URL+"/api", key, &v); err != nil || v.N != 4 {
		t.Errorf("read %+v, %v", v, err)
	}
	err := JSON(t.Context(), s.URL+"/library", key, &v)
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
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"n": 4}`))
	}))
	t.Cleanup(s.Close)
	dir := t.TempDir()
	ca := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: s.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}

	var v struct{ N int }
	if err := JSON(context.Background(), s.URL, nil, &v); err == nil || !strings.Contains(err.Error(), "certificate") {
		t.Errorf("a certificate the system doesn't trust is refused: %v", err)
	}
	c, err := Trusting(ca, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.JSON(context.Background(), s.URL, nil, &v); err != nil || v.N != 4 {
		t.Errorf("read %+v, %v", v, err)
	}
	// The test certificate is for example.com, *.example.com and 127.0.0.1.
	for name, ok := range map[string]bool{"example.com": true, "backup.example.org": false} {
		named, err := Trusting(ca, name)
		if err != nil {
			t.Fatal(err)
		}
		if err := named.JSON(context.Background(), s.URL, nil, &v); (err == nil) != ok {
			t.Errorf("a certificate checked for %s: %v", name, err)
		}
	}
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, s.URL, nil)
	if res, err := c.http.Do(req); err == nil {
		_ = res.Body.Close()
		t.Error("a POST went through")
	}

	notPEM := filepath.Join(dir, "ca.txt")
	if err := os.WriteFile(notPEM, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	for file, want := range map[string]string{notPEM: "holds no PEM certificate", filepath.Join(dir, "none.pem"): "no such file"} {
		if _, err := Trusting(file, ""); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: want an error with %q, got %v", file, want, err)
		}
	}
}

func TestCheckURL(t *testing.T) {
	t.Parallel()
	for raw, ok := range map[string]bool{
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
