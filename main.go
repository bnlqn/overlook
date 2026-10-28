// Overlook serves one page that tells a homelab's owner what is wrong, how
// each area is, and what the homelab did today and this week. It reads the
// sources the config names, with their keys, and changes nothing.
package main

import (
	"embed"
	"encoding/json"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

//go:embed web
var embedded embed.FS

func main() {
	configPath := flag.String("config", "overlook.yaml", "the config file")
	listen := flag.String("listen", ":8080", "the address to serve the page on")
	flag.Parse()
	config, err := loadConfig(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("overlook: serving on %s", *listen)
	log.Fatal(http.ListenAndServe(*listen, handler(config, filepath.Dir(*configPath))))
}

// handler serves the page, its snapshot, and from the config's folder the
// owner's own icons/ and custom.css. Nothing else of that folder is served.
func handler(config Config, dir string) http.Handler {
	web, _ := fs.Sub(embedded, "web")
	own := os.DirFS(filepath.Join(dir, "icons"))
	snaps := &snapshots{config: config, sources: sources(config)}
	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServerFS(web))
	for _, area := range Areas {
		mux.HandleFunc("GET /"+area, func(w http.ResponseWriter, r *http.Request) {
			http.ServeFileFS(w, r, web, "area.html")
		})
	}
	mux.HandleFunc("GET /snapshot", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(snaps.get())
	})
	mux.HandleFunc("GET /icons/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if _, err := fs.Stat(own, name); err == nil {
			http.ServeFileFS(w, r, own, name)
			return
		}
		http.ServeFileFS(w, r, web, "icons/"+name)
	})
	mux.HandleFunc("GET /custom.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css")
		if css, err := os.ReadFile(filepath.Join(dir, "custom.css")); err == nil {
			w.Write(css)
		}
	})
	return mux
}
