// Overlook serves one page that tells a homelab's owner what is wrong, how
// each area is, and what the homelab did today and this week. It reads the
// sources overlook.yaml names, with their keys, and changes nothing.
//
// Usage:
//
//	overlook [-config overlook.yaml] [-listen :8080]
package main

import (
	"context"
	"errors"
	"flag"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	// Today starts at midnight in TZ, which needs the zone database: built
	// in, so it works in an image that has none.
	_ "time/tzdata"

	"github.com/bnlqn/overlook/internal/config"
	"github.com/bnlqn/overlook/internal/server"
	"github.com/bnlqn/overlook/internal/snapshot"
)

func main() {
	if err := run(); err != nil {
		slog.Error("overlook stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	path := flag.String("config", "overlook.yaml", "the config file; icons/ and custom.css beside it are served too")
	listen := flag.String("listen", ":8080", "the address to serve the page on")
	flag.Parse()

	handler, sources, err := open(*path)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Addr:              *listen,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		// Long enough for a snapshot, which reads each source for at most 10 s.
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  2 * time.Minute,
	}

	// A container stops with SIGTERM: finish what's being served, then stop.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	served := make(chan error, 1)
	go func() { served <- srv.ListenAndServe() }()
	slog.Info("overlook is serving", "version", snapshot.Version, "address", *listen, "sources", sources)
	select {
	case err := <-served:
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdown); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// open serves the page for the config at path, and says how many sources it
// reads; or, while there is no config at all, the setup page (ADR 0014). A
// config that is there but wrong stops Overlook, and says why.
func open(path string) (http.Handler, int, error) {
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		// The setup page asks for the code before it tries a source, so that
		// only someone who can read this log can have Overlook read with it.
		code := server.Code()
		slog.Info("no config, so the page is the setup page until there is one, and asks for this code", "config", path, "code", code)
		h, err := server.Setup(code)
		return h, 0, err
	}
	c, err := config.Load(path)
	if err != nil {
		return nil, 0, err
	}
	h, err := server.New(c, filepath.Dir(path))
	return h, len(c.Readers()), err
}
