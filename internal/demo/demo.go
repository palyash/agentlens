// Package demo bundles synthetic transcripts so the dashboard can be tried without any real data.
package demo

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/palyash/agentlens/internal/transcript"
)

//go:embed all:data
var data embed.FS

// Latest timestamp in the bundled fixtures.
var fixtureEnd = time.Date(2026, 10, 25, 13, 0, 0, 0, time.UTC)

// Extract writes the sample transcripts to a temp dir and shifts their timestamps to end now.
func Extract() (string, error) {
	dir, err := os.MkdirTemp("", "agentlens-demo-")
	if err != nil {
		return "", err
	}
	transcript.Shift = time.Since(fixtureEnd)
	return dir, fs.WalkDir(data, "data", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := data.ReadFile(p)
		if err != nil {
			return err
		}
		dst := filepath.Join(dir, p[len("data/"):])
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0o644)
	})
}
