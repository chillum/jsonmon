package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

// Global started and last modified date for HTTP caching.
var modified string
var started string
var modHTML string
var modAngular string
var modJS string
var modCSS string

// Construct the last modified string.
func etag(ts time.Time) string {
	return "W/\"" + strconv.FormatInt(ts.UnixNano(), 10) + "\""
}

func init() {
	started = etag(time.Now())
	modified = started

	cacheHTML, _ := AssetInfo("index.html")
	modHTML = cacheHTML.ModTime().UTC().Format(http.TimeFormat)
	cacheAngular, _ := AssetInfo("angular.min.js")
	modAngular = cacheAngular.ModTime().UTC().Format(http.TimeFormat)
	cacheJS, _ := AssetInfo("app.js")
	modJS = cacheJS.ModTime().UTC().Format(http.TimeFormat)
	cacheCSS, _ := AssetInfo("main.css")
	modCSS = cacheCSS.ModTime().UTC().Format(http.TimeFormat)
}

// Serve the Web UI.
func handleUI(w http.ResponseWriter, r *http.Request) {
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
func handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, r, &checks, &modified)
}

// Display application version.
func handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, r, &version, &started)
}

// Output JSON.
func writeJSON(w http.ResponseWriter, r *http.Request, data interface{}, cache *string) {
	var cached bool
	var result []byte
	h := w.Header()
	h.Set("Server", "jsonmon")
	if r.Header.Get("If-None-Match") == *cache {
		cached = true
	} else {
		h.Set("ETag", *cache)
		result, _ = json.Marshal(&data)
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
