// Package web serves the static multi-page UI. The HTML, CSS, and JS are
// embedded into the binary via embed.FS so a single `fly deploy` ships the
// whole product — no separate frontend build or CDN.
//
// Layout (under static/):
//
//	index.html, exercises.html, plans.html  — one per page
//	shared/base.css, shared/shared.js       — global tokens + helpers
//	pages/<name>.css, pages/<name>.js       — per-page styles + behavior
//
// The handler maps clean URLs to their .html files (/exercises →
// exercises.html) so the URL bar reads cleanly. Every other path falls
// through to the embedded file server, which serves /shared/* and /pages/*
// assets directly.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed static
var staticFS embed.FS

// pageRoutes maps clean URL paths to the HTML file that backs them. Add a
// new page by adding the entry here and creating the file.
var pageRoutes = map[string]string{
	"/":          "index.html",
	"/exercises": "exercises.html",
	"/plans":     "plans.html",
	"/quick":     "quick.html",
	"/history":   "history.html",
	"/nutrition": "nutrition.html",
	"/login":     "login.html",
	"/signup":    "signup.html",
	"/verify":    "verify.html",
	"/settings":  "settings.html",
	"/privacy":   "privacy.html",
	"/terms":     "terms.html",
}

// Handler returns an http.Handler that serves the embedded UI.
func Handler() http.Handler {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		// Compile-time embed should never fail at runtime; if it did, we
		// have no UI to serve and panic is the right signal.
		panic("web: " + err.Error())
	}
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Reserve API and health paths so a misregistered static asset at
		// /v1/... cannot mask a real API endpoint.
		if path == "/healthz" || path == "/v1" || strings.HasPrefix(path, "/v1/") {
			http.NotFound(w, r)
			return
		}

		// Browsers auto-request /favicon.ico — serve the SVG with the
		// proper MIME so it works in both legacy and modern engines.
		if path == "/favicon.ico" {
			data, err := fs.ReadFile(sub, "favicon.svg")
			if err == nil {
				w.Header().Set("Content-Type", "image/svg+xml")
				w.Header().Set("Cache-Control", "public, max-age=86400")
				w.Write(data)
				return
			}
		}

		// Clean-URL → file mapping. Strip a trailing slash so /exercises/
		// and /exercises route the same way.
		clean := strings.TrimRight(path, "/")
		if clean == "" {
			clean = "/"
		}
		if file, ok := pageRoutes[clean]; ok {
			serveFile(w, r, sub, file)
			return
		}
		// Dynamic route: /workout/<id> → workout.html. The page reads the
		// id from window.location.pathname itself.
		if strings.HasPrefix(clean, "/workout/") {
			serveFile(w, r, sub, "workout.html")
			return
		}

		// Everything else (shared/*, pages/*, favicon.ico, etc.) is served
		// directly by the file server.
		fileServer.ServeHTTP(w, r)
	})
}

// serveFile reads a specific file from the embedded fs and writes it as the
// response. We don't use http.ServeFileFS because it does its own URL-to-file
// translation that fights with our route table.
func serveFile(w http.ResponseWriter, r *http.Request, fsys fs.FS, name string) {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(data)
}
