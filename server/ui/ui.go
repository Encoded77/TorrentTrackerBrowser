// Package ui serves the embedded single-page app from ./build.
package ui

import (
	"bytes"
	"embed"
	"io/fs"
	"net/http"
	"strings"
	"time"
)

//go:embed all:build
var build embed.FS

// Handler serves static files from the build folder, gives hashed SvelteKit
// assets under /_app/immutable a long cache and falls back to index.html for
// every other path (client-side routing).
func Handler() http.Handler {
	sub, err := fs.Sub(build, "build")
	if err != nil {
		panic(err)
	}
	index, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		panic("ui: build/index.html missing: " + err.Error())
	}
	started := time.Now()
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p != "" && p != "index.html" && !strings.HasSuffix(p, "/") {
			if st, err := fs.Stat(sub, p); err == nil && !st.IsDir() {
				if strings.HasPrefix(p, "_app/immutable/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				} else {
					w.Header().Set("Cache-Control", "no-cache")
				}
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeContent(w, r, "index.html", started, bytes.NewReader(index))
	})
}
