// Command agentlens serves a local, read-only dashboard over Claude Code transcripts.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/palyash/agentlens/internal/demo"
	"github.com/palyash/agentlens/internal/pricing"
	"github.com/palyash/agentlens/internal/server"
	"github.com/palyash/agentlens/internal/transcript"
)

func defaultRoot() string {
	base := os.Getenv("CLAUDE_CONFIG_DIR")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".claude")
	}
	return filepath.Join(base, "projects")
}

func main() {
	root := flag.String("root", defaultRoot(), "folder containing Claude Code transcripts")
	port := flag.Int("port", 8080, "port to listen on (localhost only)")
	useDemo := flag.Bool("demo", false, "use bundled sample data instead of your transcripts")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: agentlens [-root DIR] [-port N] [-demo]\n       agentlens [-root DIR] doctor")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *useDemo {
		dir, err := demo.Extract()
		if err != nil {
			log.Fatal(err)
		}
		defer os.RemoveAll(dir)
		*root = dir
	}
	if _, err := os.Stat(*root); err != nil {
		log.Fatalf("no transcript folder at %s (use -root or -demo)", *root)
	}
	if flag.Arg(0) == "doctor" {
		doctor(*root)
		return
	}
	fmt.Printf("Reading transcripts from %s\nOpen http://127.0.0.1:%d\n", *root, *port)
	srv := &http.Server{Addr: fmt.Sprintf("127.0.0.1:%d", *port), Handler: server.New(*root), ReadHeaderTimeout: 5 * time.Second}
	log.Fatal(srv.ListenAndServe())
}

// doctor reports what was found, to diagnose format changes in the (undocumented) transcript format.
func doctor(root string) {
	ss := transcript.LoadAll(root)
	models, unpriced := map[string]int{}, map[string]bool{}
	calls, tools, errs := 0, 0, 0
	for _, s := range ss {
		for _, c := range s.Calls {
			calls++
			models[c.Model]++
			if !c.Priced {
				unpriced[c.Model] = true
			}
			for _, t := range c.Tools {
				tools++
				if t.Err {
					errs++
				}
			}
		}
	}
	fmt.Printf("root:      %s\nsessions:  %d\nrequests:  %d\ntool calls: %d (%d flagged as errors)\n", root, len(ss), calls, tools, errs)
	names := make([]string, 0, len(models))
	for m := range models {
		names = append(names, m)
	}
	sort.Strings(names)
	for _, m := range names {
		_, ok := pricing.For(m)
		note := ""
		if !ok {
			note = "  <- no price in table, cost shows as $0"
		}
		fmt.Printf("  model %-28s %5d requests%s\n", m, models[m], note)
	}
	if calls == 0 {
		fmt.Println("No requests parsed. The transcript format may have changed; check the files under the root.")
	}
}
