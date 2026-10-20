package server

import (
	"io/fs"
	"net/http"
)

const devPlaceholder = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>NandoCode dev</title></head><body><h1>NandoCode dev gallery</h1><p>No gallery page is embedded yet.</p></body></html>`

// devHandler serves the /dev/ component gallery. It answers 404 unless the
// server was started with --dev, regardless of NoUI.
func (s *Server) devHandler() http.Handler {
	if !s.cfg.Dev {
		return http.NotFoundHandler()
	}
	sub, err := fs.Sub(webUI, "web/dev")
	if err != nil {
		return http.NotFoundHandler()
	}
	files := http.StripPrefix("/dev/", staticJSContentType(http.FileServer(http.FS(sub))))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/dev" {
			http.Redirect(w, r, "/dev/", http.StatusTemporaryRedirect)
			return
		}
		if r.URL.Path == "/dev/" {
			if _, err := fs.Stat(sub, "index.html"); err != nil {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				_, _ = w.Write([]byte(devPlaceholder))
				return
			}
		}
		files.ServeHTTP(w, r)
	})
}
