package servarr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetSendsTheKey(t *testing.T) {
	t.Parallel()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "0123abcd" {
			http.Error(w, "no key", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(s.Close)
	c := Config{URL: s.URL + "/", Key: "0123abcd"}
	if err := c.Check("Radarr"); err != nil {
		t.Fatal(err)
	}
	var v []any
	if err := c.Get(context.Background(), "/api/v3/health", &v); err != nil {
		t.Error(err)
	}
}

func TestConfigCheck(t *testing.T) {
	t.Parallel()
	c := Config{URL: "http://radarr:7878", Key: "0123abcd"}
	if err := c.Check("Radarr"); err != nil || c.Name != "Radarr" {
		t.Errorf("defaults: %+v, %v", c, err)
	}
	for _, bad := range []struct {
		c    Config
		want string
	}{
		{Config{Key: "0123abcd"}, "url is missing"},
		{Config{URL: "radarr:7878", Key: "0123abcd"}, "url needs"},
		{Config{URL: "http://radarr:7878"}, "key is missing"},
	} {
		if err := bad.c.Check("Radarr"); err == nil || !strings.Contains(err.Error(), bad.want) {
			t.Errorf("%+v: want an error with %q, got %v", bad.c, bad.want, err)
		}
	}
}
