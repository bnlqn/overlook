// Package server serves the page, the snapshot it's drawn from, the page's
// English words, and, from the config's folder, the owner's own icons and
// custom.css, and, if the owner turns it on, MCP for agents; or, while there
// is no config, the setup page.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/bnlqn/overlook/internal/config"
	"github.com/bnlqn/overlook/internal/fetch"
	"github.com/bnlqn/overlook/internal/snapshot"
	"github.com/bnlqn/overlook/web"
)

// policy keeps the page to its own scripts, styles and requests, so that a
// name a source sends can never run as code. Icons may come from anywhere;
// style attributes carry the page's per-element colours and delays. Only a
// page at Overlook's own address may frame it, as Home Assistant's does, its
// sidebar showing Overlook from Home Assistant's address (ADR 0019).
const policy = "default-src 'self'; script-src 'self'; style-src 'self'; style-src-attr 'unsafe-inline'; " +
	"img-src * data:; object-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'self'"

// language is a translation's file name: its language's code.
var language = regexp.MustCompile(`^[a-z]{2,3}\.json$`)

// New serves the page for c, whose own files are in dir.
func New(c config.Config, dir string) (http.Handler, error) {
	words, err := words()
	if err != nil {
		return nil, err
	}
	reader := snapshot.NewReader(c.Readers(), c.Apps, c.Tools)
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
	mux.Handle("GET /i18n/{file}", words)
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
	if c.MCP {
		english, err := web.English()
		if err != nil {
			return nil, err
		}
		mux.Handle("POST /mcp", mcp(reader, english))
		// Nothing is streamed, so there is no stream to open with GET.
		mux.HandleFunc("GET /mcp", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "MCP is answered to POST", http.StatusMethodNotAllowed)
		})
	}
	return secure(mux), nil
}

// Setup serves the setup page, while there is no config (ADR 0014): the
// page's own files, what to ask of each adapter's sources, and a try of one.
func Setup() (http.Handler, error) {
	words, err := words()
	if err != nil {
		return nil, err
	}
	adapters, err := config.Adapters()
	if err != nil {
		return nil, err
	}
	// The version, so that the page links the README of the Overlook that
	// runs, and whether it runs on Unraid, which sets HOST_OS in every
	// container, or in Home Assistant, which gives every app a
	// SUPERVISOR_TOKEN, so that it says where to save the files there.
	asks, err := json.Marshal(map[string]any{
		"version":       snapshot.Version,
		"unraid":        strings.EqualFold(os.Getenv("HOST_OS"), "unraid"),
		"homeassistant": os.Getenv("SUPERVISOR_TOKEN") != "",
		"adapters":      adapters,
	})
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServerFS(web.FS))
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, web.FS, "setup.html")
	})
	mux.Handle("GET /i18n/{file}", words)
	mux.HandleFunc("GET /setup", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(asks)
	})
	mux.HandleFunc("POST /setup", try(adapters))
	return secure(mux), nil
}

// tried is what a try found, or why it found nothing, with what to do about
// it in the page's words: the source's name, which no other may have, and
// the page's parts, counted.
type tried struct {
	Error    string         `json:"error,omitempty"`
	Why      *snapshot.Text `json:"why,omitempty"`
	Name     string         `json:"name,omitempty"`
	Checks   int            `json:"checks"`
	Problems int            `json:"problems"`
	Sections int            `json:"sections"`
	Charts   int            `json:"charts"`
	Figures  int            `json:"figures"`
	Events   int            `json:"events"`
	Chores   int            `json:"chores"`
}

// try reads one source, as the setup page gives it, once. Only the page
// itself may ask: a website its owner visits would otherwise have Overlook
// read their network for it. ponytail: the Origin check doesn't stop DNS
// rebinding, which ADR 0014 accepts while there is no config; a code from
// the log would close it, if this page ever stays up for long.
func try(adapters []config.Adapter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if origin, err := url.Parse(r.Header.Get("Origin")); err != nil || origin.Host != r.Host {
			http.Error(w, "only Overlook's own setup page may try a source", http.StatusForbidden)
			return
		}
		var source struct {
			Adapter string         `json:"adapter"`
			Fields  map[string]any `json:"fields"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&source); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var out tried
		c, err := config.Tried(source.Adapter, source.Fields)
		if err == nil {
			// Read as a snapshot reads it, but kept whole, so that Why can tell
			// what failed.
			src := c.Readers()[0]
			ctx, cancel := context.WithTimeout(r.Context(), snapshot.Timeout)
			defer cancel()
			var p snapshot.Parts
			p, err = src.Read(ctx, time.Now())
			out.Name = src.Name
			for _, check := range p.Checks {
				out.Checks++
				if check.Problem {
					out.Problems++
				}
			}
			out.Sections, out.Charts, out.Figures, out.Events = len(p.Sections), len(p.Charts), len(p.Figures), len(p.Events)
			out.Chores = len(p.Updates) + len(p.Chores)
		}
		if err != nil {
			var a config.Adapter
			if i := slices.IndexFunc(adapters, func(a config.Adapter) bool { return a.Key == source.Adapter }); i >= 0 {
				a = adapters[i]
			}
			out.Error, out.Why = err.Error(), why(fetch.Why(err), a, source.Fields)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		if err := json.NewEncoder(w).Encode(out); err != nil {
			slog.Warn("sending a try", "error", err)
		}
	}
}

// why is what the setup page says to do about a try that failed, by its
// kind (fetch.Why), naming its adapter and the host it was given: a CA or a
// certificate's name to give where the adapter asks for one, the address of
// the machine where the host is the container's own, or where it's a .local
// name, which a container may not look up. With no kind there is nothing to
// say past the error itself, such as the config's own "key is missing".
func why(kind string, a config.Adapter, fields map[string]any) *snapshot.Text {
	asks := func(field string) bool {
		return slices.ContainsFunc(a.Asks, func(k config.Ask) bool { return k.Field == field })
	}
	host := hostOf(fields)
	key := kind
	switch kind {
	case "certificate":
		if !asks("ca") {
			key = "certificate_none"
		}
	case "certificate_name":
		if !asks("server_name") {
			key = "certificate_name_none"
		}
	case "refused":
		if host == "localhost" || net.ParseIP(host).IsLoopback() {
			key = "localhost"
		}
	case "no_host", "timeout":
		// In a container, a .local name may not fail to be found: its lookup
		// can wait out the timeout instead.
		if strings.HasSuffix(host, ".local") {
			key = "local"
		}
	case "plain", "key", "not_found":
	default:
		return nil
	}
	return &snapshot.Text{Key: "setup.why." + key, Args: map[string]any{"name": a.Key, "host": host}}
}

// hostOf is the host a source was given at, without its port: from its
// address, or from a host with or without a port, such as NUT's, whether
// upsd.example.com or [::1]:3493.
func hostOf(fields map[string]any) string {
	if s, _ := fields["url"].(string); s != "" {
		if u, err := url.Parse(s); err == nil {
			return u.Hostname()
		}
	}
	s, _ := fields["host"].(string)
	if host, _, err := net.SplitHostPort(s); err == nil {
		return host
	}
	return s
}

// words serves the page's English words, and a translation for the
// viewer's language; one with no translation yet gets no words, and the
// page uses English throughout.
func words() (http.HandlerFunc, error) {
	english, err := web.English()
	if err != nil {
		return nil, err
	}
	en, err := json.Marshal(english)
	if err != nil {
		return nil, err
	}
	return func(w http.ResponseWriter, r *http.Request) {
		file := r.PathValue("file")
		if !language.MatchString(file) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if file == "en.json" {
			_, _ = w.Write(en)
			return
		}
		translation, err := fs.ReadFile(web.FS, "i18n/"+file)
		if err != nil {
			translation = []byte("{}")
		}
		_, _ = w.Write(translation) //nolint:gosec // an embedded file, its name checked above
	}, nil
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
