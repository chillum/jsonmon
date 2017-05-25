package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

// Construct the last modified string.
func etag(ts time.Time) string {
	return "W/\"" + strconv.FormatInt(ts.UnixNano(), 10) + "\""
}

// Serve the Web UI.
func getUI(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Server", "jsonmon")
	switch r.URL.Path {
	case "/":
		displayUI(w, r, "text/html", "index.html", &modHTML)
	case "/angular.min.js":
		displayUI(w, r, "application/javascript", "angular.min.js", &modAngular)
	case "/app.js":
		displayUI(w, r, "application/javascript", "app.js", &modJS)
	case "/main.css":
		displayUI(w, r, "text/css", "main.css", &modCSS)
	default:
		http.NotFound(w, r)
	}
}

// Web UI caching and delivery.
func displayUI(w http.ResponseWriter, r *http.Request, mime string, name string, modified *string) {
	if cached := r.Header.Get("If-Modified-Since"); cached == *modified {
		w.WriteHeader(http.StatusNotModified)
	} else {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Last-Modified", *modified)
		h.Set("Content-Type", mime)
		data, _ := Asset(name)
		w.Write(data)
	}
}

// Display checks' details.
func getChecks(w http.ResponseWriter, r *http.Request) {
	displayJSON(w, r, &checks, &modified, true)
}

// Display application version.
func getVersion(w http.ResponseWriter, r *http.Request) {
	displayJSON(w, r, &version, &started, false)
}

// Output JSON.
func displayJSON(w http.ResponseWriter, r *http.Request, data interface{}, cache *string, lock bool) {
	var cached bool
	var result []byte
	h := w.Header()
	h.Set("Server", "jsonmon")
	if lock {
		mutex.RLock()
	}
	if r.Header.Get("If-None-Match") == *cache {
		cached = true
	} else {
		h.Set("ETag", *cache)
		result, _ = json.Marshal(&data)
	}
	if lock {
		mutex.RUnlock()
	}
	if cached {
		w.WriteHeader(http.StatusNotModified)
	} else {
		h.Set("Cache-Control", "no-cache")
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Content-Type", "application/json; charset=utf-8")
		w.Write(result)
	}
}
