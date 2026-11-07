package adaptertest

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"path"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/snapshot"
)

// Bend reads the answers recorded in dir with read, once for each odd answer
// a source may give, and fails t for each that makes the adapter give back
// nothing for its source: an error, or a panic. One odd answer may cost a
// row, never its source, as one entity's time once took all of Home
// Assistant off the page (#102).
//
// An odd answer is one of dir's JSON answers with one item of one of its
// lists bent: a field set to null or left out, the item itself null, or a
// time written in another format, as 2026-03-16 16:24:44 for
// 2026-03-16T16:24:44Z, or the other way round. Each bend is made to the
// first item at its place in the answer that it changes. Such an answer may
// lead the adapter to read less, or at a path with no answer recorded, which
// is a 404, or with a query none was recorded for, which gets the one
// recorded at that path for the fewest pairs.
//
// read reads the source at url as the adapter's test does: it fills the
// adapter's Config, checks it and reads it, at the moment the answers were
// recorded.
func Bend(t *testing.T, dir string, read func(url string) (snapshot.Parts, error)) {
	t.Helper()
	files := load(t, dir, nil)
	// Odd answers are played four at a time, each by a server that plays
	// one at a time. Go keeps only two connections to a server open for the
	// next read, so a server for each, or one for all at once, would take
	// thousands of the machine's ports.
	players := make(chan *player, 4)
	for range cap(players) {
		p := &player{}
		server := httptest.NewServer(p)
		t.Cleanup(server.Close)
		p.url = server.URL
		players <- p
	}
	try := func(t *testing.T, files map[string]string) error {
		t.Helper()
		p := <-players
		defer func() { players <- p }()
		p.mu.Lock()
		p.playing = source(t, files, false)
		p.mu.Unlock()
		_, err := safely(read, p.url)
		return err
	}

	if err := try(t, files); err != nil {
		t.Fatalf("the answers as recorded give back nothing: %v", err)
	}
	all := bends(files)
	if len(all) == 0 {
		t.Fatal("no list in the answers to bend")
	}
	for _, b := range all {
		t.Run(b.name+" "+b.how, func(t *testing.T) {
			t.Parallel()
			bent := maps.Clone(files)
			bent[b.name] = b.text()
			if err := try(t, bent); err != nil {
				t.Errorf("gives back nothing for its source: %v", err)
			}
		})
	}
}

// player is a server that plays one source at a time.
type player struct {
	url     string
	mu      sync.Mutex
	playing *Source
}

func (p *player) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	s := p.playing
	p.mu.Unlock()
	s.serve(w, r)
}

// safely is read's answer for url, with a panic as its error.
func safely(read func(string) (snapshot.Parts, error), url string) (p snapshot.Parts, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v\n%s", r, debug.Stack())
		}
	}()
	return read(url)
}

// bend is one odd answer: the answer in the file name, with one item of a
// list bent as how says. text writes it when it's read, so that thousands
// of them aren't kept at once.
type bend struct {
	name, how string
	text      func() string
}

// bends gives back every odd answer Bend reads, from files, the answers by
// their files' names. A file whose answer isn't JSON, such as one in
// iCalendar, gives none.
func bends(files map[string]string) []bend {
	var all []bend
	// Each answer is read once, and each bend made to it in place and undone
	// once it's written, one at a time.
	var mu sync.Mutex
	for _, name := range slices.Sorted(maps.Keys(files)) {
		head, body := "", files[name]
		if path.Ext(name) == ".http" {
			// A whole answer's head ends at its first blank line.
			_, after, ok := strings.Cut(body, "\r\n\r\n")
			if !ok {
				_, after, ok = strings.Cut(body, "\n\n")
			}
			if !ok {
				continue
			}
			head, body = body[:len(body)-len(after)], after
		}
		var root any
		d := json.NewDecoder(strings.NewReader(body))
		d.UseNumber()
		if d.Decode(&root) != nil {
			continue
		}
		// done is each bend made, by its place in the answer and how, so
		// that each is made once, to the first item it changes.
		done := map[string]bool{}
		bent := func(at, how string, set, undo func()) {
			if done[at+" "+how] {
				return
			}
			done[at+" "+how] = true
			all = append(all, bend{name: name, how: at + " " + how, text: func() string {
				mu.Lock()
				defer mu.Unlock()
				set()
				defer undo()
				b, err := json.Marshal(root)
				if err != nil {
					panic(err) // what was read from JSON is always written back
				}
				return head + string(b)
			}})
		}
		var walk func(v any, at string, inList bool)
		walk = func(v any, at string, inList bool) {
			switch x := v.(type) {
			case map[string]any:
				for _, k := range slices.Sorted(maps.Keys(x)) {
					was, at := x[k], at+"."+k
					if inList {
						back := func() { x[k] = was }
						if was != nil {
							bent(at, "null", func() { x[k] = nil }, back)
						}
						bent(at, "left out", func() { delete(x, k) }, back)
						if s, ok := retimed(was); ok {
							bent(at, "in another format", func() { x[k] = s }, back)
						}
					}
					walk(was, at, inList)
				}
			case []any:
				at += "[*]"
				for i, item := range x {
					back := func() { x[i] = item }
					if item != nil {
						bent(at, "null", func() { x[i] = nil }, back)
					}
					if s, ok := retimed(item); ok {
						bent(at, "in another format", func() { x[i] = s }, back)
					}
					walk(item, at, true)
				}
			}
		}
		walk(root, "", false)
	}
	return all
}

// retimed is v, if it's a time, written in another format: one in RFC 3339
// as some sources write theirs, 2026-03-16 16:24:44, and one written so as
// 2026-03-16T16:24:44Z.
func retimed(v any) (string, bool) {
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.Format(time.DateTime), true
	}
	if t, err := time.Parse(time.DateTime, s); err == nil {
		return t.Format(time.RFC3339), true
	}
	return "", false
}
