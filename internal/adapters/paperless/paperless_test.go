package paperless

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
)

// testdata holds answers recorded from a scratch Paperless-ngx 3.3.4 (ADR 0008),
// which testdata/fill.sh fills: ten invented letters, three of them in the
// inbox, and a PDF cut short, which failed. Its classifier hasn't trained
// and its sanity check hasn't run, as on any new Paperless-ngx.
func TestRead(t *testing.T) {
	t.Parallel()
	src := adaptertest.Replay(t, "testdata")
	c := Config{URL: src.URL, Token: "0123abcd"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	p, err := Read(context.Background(), c, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, p)

	if len(p.Sections) != 1 || p.Sections[0].Title.Key != "paperless.documents" {
		t.Fatalf("one section: %+v", p.Sections)
	}
	rows := p.Sections[0].Rows
	if len(rows) != 6 {
		t.Fatalf("the documents, the space, two parts not well, the failed scan and the version: %+v", rows)
	}
	if r := rows[0]; r.Value.Args["n"] != 10 || r.Detail.Args["n"] != 3 {
		t.Errorf("10 documents, 3 in the inbox: %+v %+v", r.Value, r.Detail)
	}
	if r := rows[1]; r.Meter == nil || r.Value.Args["n"] != int64(915_525_713_920) {
		t.Errorf("the room left: %+v", r)
	}
	if r := rows[2]; r.Title.Key != "paperless.classifier" || r.Tone != "orange" || r.Detail.Name != "No classifier training tasks found" {
		t.Errorf("the classifier, untrained, as Paperless-ngx words it: %+v", r)
	}
	if r := rows[3]; r.Title.Key != "paperless.sanity_check" || r.Tone != "orange" {
		t.Errorf("the sanity check, never run: %+v", r)
	}
	if r := rows[4]; r.Title.Name != "scan.pdf" || r.Tone != "red" || !strings.Contains(r.Detail.Name, "InputFileError") ||
		r.Value.Args["at"] != time.Date(2026, 10, 29, 11, 48, 54, 922048000, time.UTC) {
		t.Errorf("the scan it couldn't take in, with why: %+v %+v", r, r.Value)
	}
	if r := rows[5]; r.Value.Name != "3.3.4" {
		t.Errorf("the version: %+v", r)
	}
	if f := p.Figures[0]; f.Value != 10 || f.Detail.Args["n"] != 3 || f.Tone != "orange" {
		t.Errorf("10 documents, 3 in the inbox, orange for the failure: %+v", f)
	}
}

// Overlook asks for the API's version 10, so that a newer default doesn't
// change the answers it reads.
func TestVersion(t *testing.T) {
	t.Parallel()
	var accept string
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accept = r.Header.Get("Accept")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(src.Close)
	c := Config{URL: src.URL, Token: "0123abcd"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	if err := c.source.JSON(context.Background(), "/api/status/", new(struct{})); err != nil {
		t.Fatal(err)
	}
	if accept != "application/json; version=10" {
		t.Errorf("asked for %q", accept)
	}
}

func TestConfigCheck(t *testing.T) {
	t.Parallel()
	c := Config{URL: "http://paperless:8000", Token: "0123abcd"}
	if err := c.Check(); err != nil || c.Name != "Paperless-ngx" {
		t.Errorf("defaults: %+v, %v", c, err)
	}
	for _, bad := range []struct {
		c    Config
		want string
	}{
		{Config{Token: "0123abcd"}, "url is missing"},
		{Config{URL: "paperless:8000", Token: "0123abcd"}, "url needs"},
		{Config{URL: "http://paperless:8000"}, "token is missing"},
	} {
		if err := bad.c.Check(); err == nil || !strings.Contains(err.Error(), bad.want) {
			t.Errorf("%+v: want an error with %q, got %v", bad.c, bad.want, err)
		}
	}
}
