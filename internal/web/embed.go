// Package web embeds the dashboard's static files.
package web

import "embed"

//go:embed index.html app.js style.css
var FS embed.FS
