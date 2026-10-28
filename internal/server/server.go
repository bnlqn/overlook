// Package server serves the page, the snapshot it's drawn from, the page's
// English words, and, from the config's folder, the owner's own icons and
// custom.css.
package server

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bnlqn/overlook/internal/config"
	"github.com/bnlqn/overlook/internal/snapshot"
	"github.com/bnlqn/overlook/web"
)

// policy keeps the page to its own scripts, styles and requests, so that a
// name a source sends can never run as code. Icons may come from anywhere;
// style attributes carry the page's per-element colours and delays.
const policy = "default-src 'self'; script-src 'self'; style-src 'self'; style-src-attr 'unsafe-inline'; " +
	"img-src * data:; object-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

// language is a translation's file name: its language's code.
var language = regexp.MustCompile(`^[a-z]{2,3}\.json$`)

// New serves the page for c, whose own files are in dir.
func New(c config.Config, dir string) (http.Handler, error) {
	english, err := web.English()
	if err != nil {
		return nil, err
	}
	words, err := json.Marshal(english)
	if err != nil {
		return nil, err
	}
	reader := snapshot.NewReader(c.Readers(), c.Apps, c.Tools, c.Chores)
	own := os.DirFS(filepath.Join(dir, "icons"))

	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServerFS(web.FS))
	for _, area := range snapshot.Areas {
		mux.HandleFunc("GET /"+area, func(w http.ResponseWriter, r *http.Request) {
			http.ServeFileFS(w, r, web.FS, "area.html")
		})
	}
	mux.HandleFunc("GET /snapshot", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		if err := json.NewEncoder(w).Encode(reader.Snapshot()); err != nil {
			slog.Warn("sending the snapshot", "error", err)
		}
	})
	// The page asks for its viewer's language; one with no translation yet
	// gets no words, and the page uses English throughout.
	mux.HandleFunc("GET /i18n/{file}", func(w http.ResponseWriter, r *http.Request) {
		file := r.PathValue("file")
		if !language.MatchString(file) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if file == "en.json" {
			_, _ = w.Write(words)
			return
		}
		translation, err := fs.ReadFile(web.FS, "i18n/"+file)
		if err != nil {
			translation = []byte("{}")
		}
		_, _ = w.Write(translation) //nolint:gosec // an embedded file, its name checked above
	})
	mux.HandleFunc("GET /icons/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if !fs.ValidPath(name) || strings.Contains(name, "/") {
			http.NotFound(w, r)
			return
		}
		if _, err := fs.Stat(own, name); err == nil {
			http.ServeFileFS(w, r, own, name) //nolint:gosec // one valid file name, in icons/
			return
		}
		http.ServeFileFS(w, r, web.FS, "icons/"+name) //nolint:gosec // one valid file name, in icons/
	})
	mux.HandleFunc("GET /custom.css", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		css, err := os.ReadFile(filepath.Join(dir, "custom.css")) //nolint:gosec // beside the owner's config
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			slog.Warn("reading custom.css", "error", err)
		}
		_, _ = w.Write(css)
	})
	return secure(mux), nil
}

// secure adds the headers every answer carries.
func secure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", policy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}
