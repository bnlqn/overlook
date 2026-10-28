package servarr

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCache(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.UTC)
	c := Cache[int]{Now: func() time.Time { return now }}
	reads := 0
	fail := false
	read := func(context.Context) (int, error) {
		reads++
		if fail {
			return 0, errors.New("timeout")
		}
		return reads, nil
	}
	get := func() (int, error) { return c.Get(context.Background(), read) }

	if v, err := get(); v != 1 || err != nil {
		t.Fatalf("the first read: %d, %v", v, err)
	}
	now = now.Add(every - time.Second)
	if v, _ := get(); v != 1 || reads != 1 {
		t.Errorf("an answer under ten minutes old is kept: %d after %d reads", v, reads)
	}
	now = now.Add(time.Second)
	if v, _ := get(); v != 2 || reads != 2 {
		t.Errorf("an answer ten minutes old is read again: %d after %d reads", v, reads)
	}
	now = now.Add(every)
	fail = true
	if _, err := get(); err == nil {
		t.Error("a failed read is an error, not the old answer")
	}
	if _, err := get(); err == nil || reads != 4 {
		t.Errorf("a failed read isn't kept: %d reads", reads)
	}
}

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
