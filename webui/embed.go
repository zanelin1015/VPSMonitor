package webui

import (
	"bytes"
	"embed"
	"html"
	"io/fs"
	"net/http"
	"path"
	"regexp"
	"strings"
	"time"
)

//go:embed dist/*
var distFS embed.FS

var versionedAssetPath = regexp.MustCompile(`^assets/[^/]+-[A-Za-z0-9_-]{8}\.(js|css)$`)

func NewHandler(basePaths ...func(*http.Request) string) http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return http.NotFoundHandler()
	}

	return newHandler(sub, basePaths...)
}

func newHandler(files fs.FS, basePaths ...func(*http.Request) string) http.Handler {
	fileServer := http.FileServer(http.FS(files))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		basePath := ""
		if len(basePaths) > 0 && basePaths[0] != nil {
			basePath = basePaths[0](r)
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "." || name == "" {
			serveIndexHTML(w, r, files, basePath)
			return
		}

		if fileExists(files, name) {
			// Vite changes these URLs when their contents change. Allow browsers
			// and CDNs to reuse bundles instead of fetching them on every visit.
			if versionedAssetPath.MatchString(name) {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			req := r.Clone(r.Context())
			req.URL.Path = "/" + name
			fileServer.ServeHTTP(w, req)
			return
		}

		// A stale bundle URL must not return the SPA HTML (or cache it as JS).
		if name == "assets" || strings.HasPrefix(name, "assets/") {
			w.Header().Set("Cache-Control", "no-store")
			http.NotFound(w, r)
			return
		}

		serveIndexHTML(w, r, files, basePath)
	})
}

func fileExists(files fs.FS, name string) bool {
	file, err := files.Open(name)
	if err != nil {
		return false
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func serveIndexHTML(w http.ResponseWriter, r *http.Request, files fs.FS, basePath string) {
	body, err := fs.ReadFile(files, "index.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	base := html.EscapeString(strings.TrimRight(basePath, "/") + "/")
	body = bytes.Replace(body, []byte("<head>"), []byte("<head><base href=\""+base+"\">"), 1)
	// Entry HTML must not be shared between root and prefix deployments.
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(body))
}
