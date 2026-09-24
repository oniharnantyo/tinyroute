package dashboard

import (
	"embed"
	"io"
	"io/fs"
	"net/http"
	"strings"

	"github.com/oniharnantyo/tinyroute/internal/dashboard/assets"
)

//go:embed all:dist
var distFS embed.FS

// DistFS returns the embedded filesystem of the built Vite dashboard.
func DistFS() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}

// SPAHandler returns an http.Handler that serves the Vite Single Page Application and dashboard static assets.
func SPAHandler() http.Handler {
	sub, _ := fs.Sub(distFS, "dist")
	legacyFS := assets.FS()

	var fileServer http.Handler
	if sub != nil {
		fileServer = http.FileServer(http.FS(sub))
	}
	legacyServer := http.FileServer(legacyFS)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqPath := strings.TrimPrefix(r.URL.Path, "/dashboard")
		reqPath = strings.TrimPrefix(reqPath, "/")

		if reqPath == "" {
			reqPath = "index.html"
		}

		// 1. Check if the file exists in Vite dist FS
		if sub != nil {
			if f, err := sub.Open(reqPath); err == nil {
				if stat, err := f.Stat(); err == nil && !stat.IsDir() {
					f.Close()
					http.StripPrefix("/dashboard/", fileServer).ServeHTTP(w, r)
					return
				}
				f.Close()
			}
		}

		// 2. Check if the file exists in legacy assets FS (e.g. dialog.js, styles.css)
		legacyPath := strings.TrimPrefix(reqPath, "assets/")
		if legacyPath == "" {
			legacyPath = reqPath
		}
		if f, err := legacyFS.Open(legacyPath); err == nil {
			if stat, err := f.Stat(); err == nil && !stat.IsDir() {
				f.Close()
				http.StripPrefix("/dashboard/assets/", legacyServer).ServeHTTP(w, r)
				return
			}
			f.Close()
		}

		// 3. For asset-specific extensions or asset prefixes, return 404 if not found
		if strings.HasPrefix(reqPath, "assets/") || strings.HasPrefix(reqPath, "logos/") ||
			strings.HasSuffix(reqPath, ".js") || strings.HasSuffix(reqPath, ".css") ||
			strings.HasSuffix(reqPath, ".svg") || strings.HasSuffix(reqPath, ".png") ||
			strings.HasSuffix(reqPath, ".ico") || strings.HasSuffix(reqPath, ".json") {
			http.NotFound(w, r)
			return
		}

		// 4. Client-side SPA routing fallback -> index.html
		if sub != nil {
			indexFile, err := sub.Open("index.html")
			if err == nil {
				defer indexFile.Close()
				stat, _ := indexFile.Stat()
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				if rs, ok := indexFile.(io.ReadSeeker); ok {
					http.ServeContent(w, r, "index.html", stat.ModTime(), rs)
				} else {
					io.Copy(w, indexFile)
				}
				return
			}
		}

		http.Error(w, "Dashboard frontend not found. Run 'pnpm build' in web/ directory.", http.StatusNotFound)
	})
}
