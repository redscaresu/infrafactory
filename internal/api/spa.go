package api

import (
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
)

func SPAHandler(assets fs.FS) http.Handler {
	subFS, err := fs.Sub(assets, "ui/build")
	if err != nil {
		subFS = assets
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}

		filePath := cleanPath(r.URL.Path)
		if filePath == "" {
			filePath = "index.html"
		}

		data, err := fs.ReadFile(subFS, filePath)
		if err != nil {
			data, err = fs.ReadFile(subFS, "index.html")
			if err != nil {
				http.NotFound(w, r)
				return
			}
			serveBytes(w, "index.html", data)
			return
		}

		serveBytes(w, filePath, data)
	})
}

func cleanPath(requestPath string) string {
	cleaned := path.Clean("/" + requestPath)
	cleaned = strings.TrimPrefix(cleaned, "/")
	if cleaned == "." {
		return ""
	}
	if strings.Contains(cleaned, "..") {
		return ""
	}
	return cleaned
}

func serveBytes(w http.ResponseWriter, name string, data []byte) {
	if ctype := mime.TypeByExtension(path.Ext(name)); ctype != "" {
		w.Header().Set("Content-Type", ctype)
	}
	_, _ = w.Write(data)
}
