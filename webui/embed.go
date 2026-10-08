package webui

import (
	"bytes"
	"embed"
	"html"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

//go:embed dist/*
var distFS embed.FS

func NewHandler(basePaths ...func(*http.Request) string) http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return http.NotFoundHandler()
	}

	fileServer := http.FileServer(http.FS(sub))
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
			serveIndexHTML(w, r, sub, basePath)
			return
		}

		if fileExists(sub, name) {
			req := r.Clone(r.Context())
			req.URL.Path = "/" + name
			fileServer.ServeHTTP(w, req)
			return
		}

		serveIndexHTML(w, r, sub, basePath)
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
