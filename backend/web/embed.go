// Package web holds the static frontend, compiled into the binary so the whole
// app ships as a single executable.
package web

import "embed"

// Files is the embedded static UI served by the HTTP server.
//
//go:embed index.html app.js style.css
var Files embed.FS
