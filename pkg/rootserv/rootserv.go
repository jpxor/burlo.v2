// Copyright (C) 2025 Josh Simonot
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package rootserv

import (
	"burlo/v2/pkg/logger"
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

// RootServer holds a mux and the list of attached sub-handlers.
type RootServer struct {
	log        *logger.Logger
	addr       string
	mux        *http.ServeMux
	subservers map[string]string // path -> description
	mainPage   http.Handler      // optional subserver for '/'
}

// New creates a new RootServer bound to an address.
func New(addr string) *RootServer {
	return &RootServer{
		addr:       addr,
		mux:        http.NewServeMux(),
		subservers: make(map[string]string),
		log:        logger.New("HTTPServer"),
	}
}

// Attach registers a new subserver under a path.
// If path == "/", it becomes the main page and can handle its own subpaths.
func (ms *RootServer) Attach(path, desc string, handler http.Handler) {
	ms.log.Info("Attach: %s", path)

	// Root handler special case
	if path == "/" {
		ms.mainPage = handler
		ms.log.Info("Main page registered at /")
		return
	}

	// Normalize path:
	//  - Ensure it starts with '/'
	//  - Ensure it ends with '/' for ServeMux matching semantics
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}

	ms.subservers[strings.TrimRight(path, "/")] = desc // store pretty form

	// Strip the prefix (without trailing slash) so subserver sees clean URLs.
	strip := strings.TrimRight(path, "/")
	ms.mux.Handle(path, http.StripPrefix(strip, handler))
}

// handleIndex generates the HTML index page listing all subservers.
func (ms *RootServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	fmt.Fprintln(w, "<!DOCTYPE html><html><head><title>RootServer</title></head><body>")
	fmt.Fprintln(w, "<h1>Available Sub-Servers</h1><ul>")

	paths := make([]string, 0, len(ms.subservers))
	for path := range ms.subservers {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	for _, path := range paths {
		desc := ms.subservers[path]
		fmt.Fprintf(w, `<li><a href="%s">%s</a> - %s</li>`, path, path, desc)
	}

	fmt.Fprintln(w, "</ul></body></html>")
}

// Run starts serving and blocks until the context is canceled.
func (ms *RootServer) Run(ctx context.Context) {
	ms.log.Info("Running...")

	// index page always available
	ms.mux.HandleFunc("/index", ms.handleIndex)

	// handle favicon.ico globally
	ms.mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "www/favicon.svg")
	})

	// root path '/' delegates to mainPage if defined
	ms.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Check if any subserver matches the path exactly first
		for path := range ms.subservers {
			if path != "/" && (r.URL.Path == path || strings.HasPrefix(r.URL.Path, path+"/")) {
				ms.mux.ServeHTTP(w, r) // already handled by subserver registration
				return
			}
		}

		if ms.mainPage != nil {
			ms.mainPage.ServeHTTP(w, r)
			return
		}

		// Default fallback: redirect to /index
		http.Redirect(w, r, "/index", http.StatusTemporaryRedirect)
	})

	srv := &http.Server{
		Addr:    ms.addr,
		Handler: ms.mux,
	}

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5_000_000_000) // 5s
		defer cancel()
		srv.Shutdown(shutdownCtx)
		ms.log.Info("Stopped")
	case err := <-errCh:
		ms.log.Error("Stopped: %T %+v", err, err)
	}
}
