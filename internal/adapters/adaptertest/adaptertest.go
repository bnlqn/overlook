// Package adaptertest is the contract test every adapter passes (ADR 0005):
// Replay answers an adapter's reads with answers recorded from its source,
// and Validate checks that the page can draw what the adapter made of them.
package adaptertest

import (
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/fetch"
	"github.com/bnlqn/overlook/internal/snapshot"
	"github.com/bnlqn/overlook/web"
)

// Replay starts a source that answers each GET with the answer recorded at
// dir/<path>.json, or at dir/<path>.ics for a source that answers in
// iCalendar, the query left out, and fails tb on any other method. An
// answer whose query sets it apart from another of the same path, such as
// Pi-hole's blocked top domains, is at dir/<path>@<query>.json.
func Replay(tb testing.TB, dir string) *httptest.Server {
	tb.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			tb.Errorf("%s %s: a source is only read", r.Method, r.URL.Path)
			http.Error(w, "only GET", http.StatusMethodNotAllowed)
			return
		}
		// An fs.FS refuses a path with "..", so dir is never left.
		recorded, answer := os.DirFS(dir), strings.Trim(r.URL.Path, "/")
		if _, err := fs.Stat(recorded, answer+"@"+r.URL.RawQuery+".json"); err == nil && r.URL.RawQuery != "" {
			answer += "@" + r.URL.RawQuery + ".json"
		} else if _, err := fs.Stat(recorded, answer+".ics"); err == nil {
			answer += ".ics"
		} else {
			answer += ".json"
		}
		http.ServeFileFS(w, r, recorded, answer) //nolint:gosec // see above
	}))
	tb.Cleanup(s.Close)
	return s
}

// Validate fails tb with every way the page couldn't draw p.
func Validate(tb testing.TB, p snapshot.Parts) {
	tb.Helper()
	english, err := web.English()
	if err != nil {
		tb.Fatal(err)
	}
	for _, problem := range Problems(p, english) {
		tb.Error(problem)
	}
}

var placeholder = regexp.MustCompile(`\{(\w+)(?::\w+)?\}`)

// Problems lists every way the page couldn't draw p with the English table.
func Problems(p snapshot.Parts, english map[string]any) []string {
	var bad []string
	fail := func(format string, a ...any) { bad = append(bad, fmt.Sprintf(format, a...)) }
	text := func(where string, x *snapshot.Text) {
		if x == nil {
			return
		}
		if x.Key == "" {
			if x.Name == "" {
				fail("%s: a text with neither a key nor a name", where)
			}
			return
		}
		var forms []string
		switch entry := english[x.Key].(type) {
		case string:
			forms = []string{entry}
		case map[string]any:
			for _, form := range entry {
				forms = append(forms, fmt.Sprint(form))
			}
		default:
			fail("%s: %s isn't in the English table", where, x.Key)
		}
		missing := map[string]bool{}
		for _, form := range forms {
			for _, m := range placeholder.FindAllStringSubmatch(form, -1) {
				if _, ok := x.Args[m[1]]; !ok && !missing[m[1]] {
					missing[m[1]] = true
					fail("%s: %s needs {%s}", where, x.Key, m[1])
				}
			}
		}
	}
	area := func(where, a string) {
		if !slices.Contains(snapshot.Areas, a) {
			fail("%s: area %q isn't one of %v", where, a, snapshot.Areas)
		}
	}
	tone := func(where, t string) {
		if !slices.Contains(snapshot.Tones, t) {
			fail("%s: tone %q isn't one of %q", where, t, snapshot.Tones)
		}
	}
	meter := func(where string, m snapshot.Meter) {
		text(where, m.Label)
		tone(where, m.Tone)
		if m.Total <= 0 {
			fail("%s: a meter with nothing to fill", where)
		}
	}

	// A check engine's checks are built by snapshot.CheckOf, which makes
	// these true; the demo's and a check built by hand are held to them too.
	for _, c := range p.Checks {
		w := "check " + c.Key
		if c.Key == "" || c.Name == "" || c.URL == "" {
			fail("%s: needs a key, a name and a url", w)
		}
		if c.Area != "" {
			area(w, c.Area)
		}
		if len(c.Results) > snapshot.Recent || c.Run > len(c.Results) || c.Threshold < 0 || c.Threshold > 0 && !c.Problem && c.Run >= c.Threshold {
			fail("%s: run %d of %d results, threshold %d, problem %v", w, c.Run, len(c.Results), c.Threshold, c.Problem)
		}
		if !slices.IsSortedFunc(c.Results, func(a, b snapshot.Result) int { return a.At.Compare(b.At) }) {
			fail("%s: results aren't oldest first", w)
		}
		if (c.Since != nil) != (c.Run > 0) {
			fail("%s: failing since %v, with %d failures in a row", w, c.Since, c.Run)
		}
		var going []time.Time
		for _, o := range c.Outages {
			if o.To == nil {
				going = append(going, o.From)
			} else if !o.To.After(o.From) {
				fail("%s: an outage that ends before it starts", w)
			}
		}
		if c.Problem != (len(going) > 0) || len(going) > 1 || c.Problem && (c.Since == nil || !going[0].Equal(*c.Since)) {
			fail("%s: a problem has one outage still going, from its since, and only a problem has one", w)
		}
	}
	for _, s := range p.Sections {
		w := "section " + s.Title.Key
		area(w, s.Area)
		text(w, &s.Title)
		for _, r := range s.Rows {
			text(w, &r.Title)
			text(w, r.Value)
			text(w, r.Detail)
			tone(w, r.Tone)
			if r.Meter != nil {
				meter(w, *r.Meter)
			}
			for _, m := range r.Meters {
				meter(w, m)
			}
		}
	}
	for _, c := range p.Charts {
		w := "chart " + c.Title.Key
		area(w, c.Area)
		text(w, &c.Title)
		if len(c.Series) == 0 || len(c.Series[0].Hours) < 2 {
			fail("%s: no series of two hours or more", w)
		}
		for _, s := range c.Series {
			text(w, &s.Label)
			if len(s.Hours) != len(c.Series[0].Hours) {
				fail("%s: a series of %d hours beside one of %d", w, len(s.Hours), len(c.Series[0].Hours))
			}
		}
	}
	for _, f := range p.Figures {
		w := "figure " + f.Label.Key
		area(w, f.Area)
		text(w, &f.Label)
		text(w, f.Unit)
		text(w, f.Detail)
		tone(w, f.Tone)
		if len(f.Hours) != 0 && len(f.Hours) != 24 {
			fail("%s: %d hours, not 24", w, len(f.Hours))
		}
		for _, m := range f.Meters {
			meter(w, m)
		}
		if f.Within != nil {
			text(w, &f.Within.Label)
			if len(f.Hours) != 24 || len(f.Within.Hours) != 24 {
				fail("%s: within hours it doesn't match", w)
			}
		}
		whole := 0.0
		for _, sh := range f.Shares {
			text(w, &sh.Label)
			tone(w, sh.Tone)
			for _, r := range sh.Rows {
				text(w, &r.Title)
				text(w, r.Value)
			}
			if sh.Value < 0 {
				fail("%s: a share below nothing", w)
			}
			whole += sh.Value
		}
		if len(f.Shares) > 0 && whole <= 0 {
			fail("%s: shares of nothing", w)
		}
		text(w, f.Join)
		if f.Part != nil {
			if f.Join == nil {
				fail("%s: a part of no tile", w)
			}
			text(w, &f.Part.Label)
			tone(w, f.Part.Tone)
			for _, r := range f.Part.Rows {
				text(w, &r.Title)
				text(w, r.Value)
			}
			if f.Part.Value < 0 {
				fail("%s: a part below nothing", w)
			}
		}
	}
	for _, e := range p.Events {
		w := "event " + e.Text.Key
		area(w, e.Area)
		text(w, &e.Text)
		tone(w, e.Tone)
		if e.At.IsZero() {
			fail("%s: no time", w)
		}
	}
	for _, c := range p.Chores {
		if c.Name == "" || (c.URL != "" && fetch.CheckURL(c.URL) != nil) {
			fail("chore %q: needs a name, and an http:// or https:// url if any", c.Name)
		}
	}
	for _, u := range p.Updates {
		if u.Name == "" || u.URL == "" {
			fail("update %q: needs a name and a url", u.Name)
		}
	}
	return bad
}
