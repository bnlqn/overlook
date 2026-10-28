package fetch

import (
	"context"
	"net/http"
	"net/http/httptest"
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

func TestCheckURL(t *testing.T) {
	t.Parallel()
	for raw, ok := range map[string]bool{
		"https://status.example.com":      true,
		"http://gatus:8080":               true,
		"javascript:alert(1)":             false,
		"status.example.com":              false,
		"ftp://files.example.com":         false,
		"https://":                        false,
		"https://user:pass@x.example.com": true,
	} {
		if err := CheckURL(raw); (err == nil) != ok {
			t.Errorf("%s: %v", raw, err)
		}
	}
}
