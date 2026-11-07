package adaptertest

import (
	"bufio"
	"cmp"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"net/url"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"
)

// Source is a source Replay started, at URL. It keeps every request it is
// sent, so that a test can check what an adapter sent.
type Source struct {
	URL string

	tb     testing.TB
	strict bool
	// answers are the recorded answers, by the path they answer, without
	// its leading /.
	answers map[string][]*answer

	mu   sync.Mutex
	sent []Request
}

// Request is one request a Source was sent.
type Request struct {
	Method string
	// URL is the path and query it was sent to.
	URL    *url.URL
	Header http.Header
	Body   string
}

// answer is one recorded answer, which answers a request at its path whose
// query has each of its pairs.
type answer struct {
	file  string
	pairs [][2]string
	// whole is an answer written as curl -si prints it, with its status and
	// header, which are otherwise a 200's and Go's.
	whole  bool
	status int
	header http.Header
	body   string
	// read is guarded by the Source's mu.
	read bool
}

// Replay starts a source that answers with the answers recorded in dir, with
// each of edits, given as old and new text, made to every one of them.
//
// An answer is at dir/<path>.json, or .ics for one in iCalendar, or .http
// for one whose status or headers matter, such as the cookie a login gives,
// written whole as curl -si prints it. One at dir/<path>@<pairs>.json, its
// pairs written as the address escapes them, answers only a request whose
// query has those pairs, and of a path's answers that fit a query, the one
// with the most pairs answers. A path with no answer is a 404.
//
// Replay fails tb on a request no source it plays would be sent: anything
// but a GET, except one POST, the login (ADR 0009), to a path with an
// answer. At the end of the test it fails tb on every answer nothing read,
// so that every recorded answer is one the adapter reads.
func Replay(tb testing.TB, dir string, edits ...string) *Source {
	tb.Helper()
	s := source(tb, load(tb, dir, edits), true)
	server := httptest.NewServer(http.HandlerFunc(s.serve))
	s.URL = server.URL
	tb.Cleanup(func() {
		server.Close()
		// A test that failed may have stopped before it read them all.
		if tb.Failed() {
			return
		}
		var unread []string
		for _, list := range s.answers {
			for _, a := range list {
				if !a.read {
					unread = append(unread, a.file)
				}
			}
		}
		slices.Sort(unread)
		for _, file := range unread {
			tb.Errorf("%s: an answer nothing read", file)
		}
	})
	return s
}

// load gives back every answer recorded in dir, by its file's name, with each of
// edits, given as old and new text, made to it.
func load(tb testing.TB, dir string, edits []string) map[string]string {
	tb.Helper()
	if len(edits)%2 != 0 {
		tb.Fatal("edits are old and new text, in pairs")
	}
	files := map[string]string{}
	changed := make([]bool, len(edits))
	answers := os.DirFS(dir)
	err := fs.WalkDir(answers, ".", func(name string, d fs.DirEntry, err error) error {
		ext := path.Ext(name)
		if err != nil || d.IsDir() || ext != ".json" && ext != ".ics" && ext != ".http" {
			return err
		}
		if i := strings.IndexFunc(name, func(r rune) bool { return !nameOK(r) }); i >= 0 {
			return fmt.Errorf("%s: Go's modules refuse a %q in a file name: write it as the address escapes it", name, name[i])
		}
		b, err := fs.ReadFile(answers, name)
		if err != nil {
			return err
		}
		text := string(b)
		for i := 0; i < len(edits); i += 2 {
			if strings.Contains(text, edits[i]) {
				changed[i] = true
				text = strings.ReplaceAll(text, edits[i], edits[i+1])
			}
		}
		files[name] = text
		return nil
	})
	if err != nil {
		tb.Fatal(err)
	}
	for i := 0; i < len(edits); i += 2 {
		if !changed[i] {
			tb.Fatalf("the edit of %q changes no answer", edits[i])
		}
	}
	return files
}

// source is a source that answers with files, the answers by their files'
// names, yet to be served. A strict one, which Replay serves, fails tb on a
// query with no answer recorded. One that isn't, which plays an odd answer
// that can lead an adapter to ask with another query, answers such a query
// with the answer recorded at its path for the fewest pairs.
func source(tb testing.TB, files map[string]string, strict bool) *Source {
	tb.Helper()
	s := &Source{tb: tb, strict: strict, answers: map[string][]*answer{}}
	for name, text := range files {
		ext := path.Ext(name)
		at, base := path.Split(strings.TrimSuffix(name, ext))
		base, query, _ := strings.Cut(base, "@")
		pairs, err := url.ParseQuery(query)
		if err != nil {
			tb.Fatalf("%s: %v", name, err)
		}
		a := &answer{file: name, body: text, whole: ext == ".http"}
		for k, vs := range pairs {
			for _, v := range vs {
				a.pairs = append(a.pairs, [2]string{k, v})
			}
		}
		if a.whole {
			if a.status, a.header, a.body, err = whole(text); err != nil {
				tb.Fatalf("%s: %v", name, err)
			}
		}
		s.answers[at+base] = append(s.answers[at+base], a)
	}
	return s
}

// Sent is every request the source was sent, in the order it came.
func (s *Source) Sent() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.sent)
}

func (s *Source) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	a, err := s.answer(Request{Method: r.Method, URL: r.URL, Header: r.Header, Body: string(body)})
	switch {
	case err != nil:
		s.tb.Errorf("%s %s: %v", r.Method, r.URL, err)
		http.Error(w, err.Error(), http.StatusBadRequest)
	case a == nil:
		http.NotFound(w, r)
	case a.whole:
		// The body is served as written, edits and all, so its length is
		// the server's to say.
		for k, vs := range a.header {
			if k != "Content-Length" && k != "Transfer-Encoding" {
				w.Header()[k] = vs
			}
		}
		w.WriteHeader(a.status)
		_, _ = io.WriteString(w, a.body)
	default:
		http.ServeContent(w, r, a.file, time.Time{}, strings.NewReader(a.body))
	}
}

// answer keeps r and gives back its answer, nil for a 404, or why no source
// Replay plays would be sent it.
func (s *Source) answer(r Request) (*answer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, r)
	switch r.Method {
	case http.MethodGet:
	case http.MethodPost:
		if slices.ContainsFunc(s.sent[:len(s.sent)-1], func(x Request) bool { return x.Method == http.MethodPost }) {
			return nil, errors.New("a second login, where a source logs in once (ADR 0009)")
		}
	default:
		return nil, errors.New("a source is only read")
	}
	list, q := s.answers[strings.Trim(r.URL.Path, "/")], r.URL.Query()
	var best *answer
	for _, a := range list {
		fits := !slices.ContainsFunc(a.pairs, func(p [2]string) bool { return !slices.Contains(q[p[0]], p[1]) })
		if fits && (best == nil || len(a.pairs) > len(best.pairs)) {
			best = a
		}
	}
	if best == nil && len(list) > 0 && !s.strict {
		// An odd answer can lead an adapter to ask with another query,
		// which the source would answer as it does the ones recorded.
		best = slices.MinFunc(list, func(a, b *answer) int {
			return cmp.Or(cmp.Compare(len(a.pairs), len(b.pairs)), strings.Compare(a.file, b.file))
		})
	}
	switch {
	case best == nil && len(list) > 0:
		return nil, errors.New("no answer recorded for its query")
	case best == nil && r.Method == http.MethodPost:
		return nil, errors.New("a login with no answer recorded")
	case best != nil:
		best.read = true
	}
	return best, nil
}

// whole reads an answer written as curl -si prints it: its status line, its
// header, a blank line and its body.
func whole(text string) (status int, header http.Header, body string, err error) {
	r := textproto.NewReader(bufio.NewReader(strings.NewReader(text)))
	line, err := r.ReadLine()
	if err != nil {
		return 0, nil, "", err
	}
	proto, code, _ := strings.Cut(line, " ")
	code, _, _ = strings.Cut(code, " ")
	if status, err = strconv.Atoi(code); err != nil || !strings.HasPrefix(proto, "HTTP/") {
		return 0, nil, "", fmt.Errorf("%q isn't a status line", line)
	}
	h, err := r.ReadMIMEHeader()
	if err != nil {
		return 0, nil, "", fmt.Errorf("its header: %w", err)
	}
	b, err := io.ReadAll(r.R)
	return status, http.Header(h), string(b), err
}

// nameOK says whether Go's modules take r in a file name, which leaves out
// every character Windows refuses, such as a ":".
func nameOK(r rune) bool {
	return unicode.IsLetter(r) || '0' <= r && r <= '9' || strings.ContainsRune("/!#$%&()+,-.=@[]^_{}~ ", r)
}
