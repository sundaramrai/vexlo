package dashboard

import (
	_ "embed"
	"net/http"
)

//go:embed dashboard.html
var dashboardHTML []byte

//go:embed dashboard.css
var dashboardCSS []byte

//go:embed dashboard.js
var dashboardJS []byte

func ServeHTML(w http.ResponseWriter, _ *http.Request) {
	serveAsset(w, "text/html; charset=utf-8", dashboardHTML)
}

func ServeCSS(w http.ResponseWriter, _ *http.Request) {
	serveAsset(w, "text/css; charset=utf-8", dashboardCSS)
}

func ServeJS(w http.ResponseWriter, _ *http.Request) {
	serveAsset(w, "text/javascript; charset=utf-8", dashboardJS)
}

func serveAsset(w http.ResponseWriter, contentType string, body []byte) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	_, _ = w.Write(body)
}
