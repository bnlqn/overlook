// Demo writes the demo: the page as static files, drawn from the invented
// snapshots beside this file instead of a server's, for GitHub Pages to serve.
// With -serve, it then serves them the way GitHub Pages does, to look at.
//
// Usage:
//
//	go run ./demo [-out _site] [-serve localhost:8000]
package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/bnlqn/overlook/internal/snapshot"
	"github.com/bnlqn/overlook/web"
)

// invented is a snapshot as Overlook would read it from a homelab that
// doesn't exist, with every adapter's parts (ADR 0003).
//
//go:embed snapshot.json
var invented []byte

// The two other homelabs every change to the page is looked at with (the
// README's road to v0.2), in the shapes today's adapters give: a small one,
// Gatus and two apps, and a large one, at the most the page is made for.
var (
	//go:embed small.json
	small []byte
	//go:embed large.json
	large []byte
)

// homelabs are the demo's snapshots by the folder each is drawn in: its own
// at the root, the others at /small/ and /large/, with their areas' pages
// beside them, such as /large/servers.
var homelabs = map[string][]byte{"": invented, "small": small, "large": large}

func main() {
	out := flag.String("out", "_site", "the folder to write the demo to")
	serve := flag.String("serve", "", "an address to serve the demo on once it's written, such as localhost:8000")
	flag.Parse()
	if err := write(*out); err != nil {
		log.Fatal(err)
	}
	if *serve == "" {
		return
	}
	site := http.Dir(*out)
	files := http.FileServer(site)
	srv := &http.Server{
		Addr: *serve,
		// As on GitHub Pages, /servers is servers.html, and /large, with no
		// large.html, is the folder /large/.
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if page := r.URL.Path + ".html"; path.Ext(r.URL.Path) == "" {
				if f, err := site.Open(page); err == nil {
					_ = f.Close()
					r.URL.Path = page
				}
			}
			files.ServeHTTP(w, r)
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("the demo is at http://%s, the small and the large homelab at /small/ and /large/", *serve)
	log.Fatal(srv.ListenAndServe())
}

// write puts each homelab in its folder of out.
func write(out string) error {
	for dir, snap := range homelabs {
		if err := draw(filepath.Join(out, dir), snap); err != nil {
			return err
		}
	}
	return nil
}

// draw puts in out what the server would serve, with snap as a file and every
// page marked as the demo's.
func draw(out string, snap []byte) error {
	put := func(name string, data []byte) error {
		file := filepath.Join(out, filepath.FromSlash(name))
		// A site anyone may read.
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil { //nolint:gosec // see above
			return err
		}
		return os.WriteFile(file, data, 0o644) //nolint:gosec // see above
	}
	err := fs.WalkDir(web.FS, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(web.FS, name)
		if err != nil {
			return err
		}
		if path.Ext(name) == ".html" {
			if data, err = demo(data); err != nil {
				return err
			}
		}
		return put(name, data)
	})
	if err != nil {
		return err
	}

	// Each area's page under its name, which GitHub Pages serves at /servers
	// and the others, as the server does.
	area, err := fs.ReadFile(web.FS, "area.html")
	if err != nil {
		return err
	}
	if area, err = demo(area); err != nil {
		return err
	}
	for _, a := range snapshot.Areas {
		if err := put(a+".html", area); err != nil {
			return err
		}
	}

	english, err := web.English()
	if err != nil {
		return err
	}
	words, err := json.Marshal(english)
	if err != nil {
		return err
	}
	if err := put("i18n/en.json", words); err != nil {
		return err
	}
	if err := put("snapshot.json", snap); err != nil {
		return err
	}
	// The page links an owner's custom.css, and the demo has none.
	return put("custom.css", nil)
}

// demo marks a page as the demo's, which the page reads to draw snapshot.json
// as if it had just been read (web/js/shell.js).
func demo(page []byte) ([]byte, error) {
	marked := bytes.Replace(page, []byte("<html "), []byte("<html data-demo "), 1)
	if bytes.Equal(marked, page) {
		return nil, errors.New("a page without <html")
	}
	return marked, nil
}
