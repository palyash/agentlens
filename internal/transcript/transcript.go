// Package transcript reads Claude Code session transcripts (JSONL) into sessions.
// The on-disk format is undocumented, so decoding is deliberately tolerant.
package transcript

import (
	"bufio"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/palyash/agentlens/internal/pricing"
)

// Shift offsets every parsed timestamp. The demo uses it to keep bundled data recent.
var Shift time.Duration

type Tool struct {
	ID, Name, Arg, Fam string
	Chars              int // size of the result fed back into context
	Err, Den           bool
}

type Call struct { // one model request
	TS, Model                    string
	In, Out, CR, CW5, CW1, Think int // Think is thinking text length in characters
	Tools                        []Tool
	Cost                         float64
	Priced                       bool
}

func (c Call) CW() int { return c.CW5 + c.CW1 }

type Session struct {
	ID, Project, Branch, Prompt string
	Calls                       []Call
}

type usage struct {
	In  int `json:"input_tokens"`
	Out int `json:"output_tokens"`
	CR  int `json:"cache_read_input_tokens"`
	CW  int `json:"cache_creation_input_tokens"`
	CC  *struct {
		E5 int `json:"ephemeral_5m_input_tokens"`
		E1 int `json:"ephemeral_1h_input_tokens"`
	} `json:"cache_creation"`
}

type block struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	Input     map[string]any  `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
}

type line struct {
	Type      string          `json:"type"`
	TS        string          `json:"timestamp"`
	CWD       string          `json:"cwd"`
	Branch    string          `json:"gitBranch"`
	SessionID string          `json:"sessionId"`
	UUID      string          `json:"uuid"`
	IsMeta    bool            `json:"isMeta"`
	Denial    json.RawMessage `json:"toolDenialKind"`
	Message   struct {
		ID      string          `json:"id"`
		Model   string          `json:"model"`
		Content json.RawMessage `json:"content"`
		Usage   *usage          `json:"usage"`
	} `json:"message"`
}

type result struct {
	chars    int
	err, den bool
}

var (
	cdRe  = regexp.MustCompile(`^(cd\s+\S+\s*(&&|;)\s*)+`)
	envRe = regexp.MustCompile(`^(\w+=\S+\s+)+`)
)

func trunc(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

func family(name, raw string) string {
	if name != "Bash" {
		return ""
	}
	w := strings.Fields(envRe.ReplaceAllString(raw, ""))
	if len(w) == 0 {
		return "?"
	}
	f := filepath.Base(w[0])
	switch f {
	case "git", "go", "npm", "docker", "kubectl", "make", "terraform", "aws", "gh":
		if len(w) > 1 && !strings.HasPrefix(w[1], "-") {
			f += " " + w[1]
		}
	}
	return f
}

func firstString(in map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := in[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func shift(ts string) string {
	if Shift == 0 {
		return ts
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return ts
	}
	return t.Add(Shift).UTC().Format("2006-01-02T15:04:05.000Z")
}

// Parse reads one transcript file. Streamed messages appear on several lines with the
// same message id; they are merged, keeping the final usage numbers.
func Parse(path string) (*Session, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var (
		calls                    []Call
		idx                      = map[string]int{}
		results                  = map[string]result{}
		cwd, branch, sid, prompt string
	)
	r := bufio.NewReaderSize(f, 1<<20) // ReadBytes has no line-length cap, unlike bufio.Scanner
	for {
		b, rerr := r.ReadBytes('\n')
		var l line
		var te *json.UnmarshalTypeError
		if len(b) > 0 {
			if err := json.Unmarshal(b, &l); err == nil || errors.As(err, &te) {
				cwd, branch, sid = pick(cwd, l.CWD), pick(branch, l.Branch), pick(sid, l.SessionID)
				switch l.Type {
				case "user":
					var s string
					if json.Unmarshal(l.Message.Content, &s) == nil {
						if prompt == "" && !l.IsMeta {
							prompt = trunc(strings.TrimSpace(s), 90)
						}
						break
					}
					var bs []block
					if json.Unmarshal(l.Message.Content, &bs) != nil {
						break
					}
					den := len(l.Denial) > 0 && string(l.Denial) != "null"
					for _, x := range bs {
						switch {
						case x.Type == "tool_result":
							results[x.ToolUseID] = result{len(x.Content), x.IsError, den}
						case x.Type == "text" && prompt == "":
							prompt = trunc(strings.TrimSpace(x.Text), 90)
						}
					}
				case "assistant":
					u := l.Message.Usage
					if u == nil || l.Message.Model == "<synthetic>" {
						break
					}
					key := pick(l.Message.ID, l.UUID)
					i, ok := idx[key]
					if !ok {
						calls = append(calls, Call{TS: shift(l.TS), Model: l.Message.Model})
						i = len(calls) - 1
						idx[key] = i
					}
					c := &calls[i]
					if u.Out >= c.Out {
						c.In, c.Out, c.CR, c.CW5, c.CW1 = u.In, u.Out, u.CR, u.CW, 0
						if u.CC != nil && u.CC.E5+u.CC.E1 > 0 {
							c.CW5, c.CW1 = u.CC.E5, u.CC.E1
						}
					}
					var bs []block
					_ = json.Unmarshal(l.Message.Content, &bs)
					for _, x := range bs {
						switch x.Type {
						case "thinking":
							c.Think = max(c.Think, utf8.RuneCountInString(x.Thinking))
						case "tool_use":
							if hasTool(c.Tools, x.ID) {
								continue
							}
							raw := firstString(x.Input, "file_path", "command", "pattern")
							if x.Name == "Bash" {
								raw = cdRe.ReplaceAllString(strings.TrimSpace(raw), "")
							}
							c.Tools = append(c.Tools, Tool{ID: x.ID, Name: pick(x.Name, "?"), Arg: trunc(raw, 80), Fam: family(x.Name, raw)})
						}
					}
				}
			}
		}
		if rerr != nil {
			break
		}
	}
	for i := range calls {
		c := &calls[i]
		for j := range c.Tools {
			t, rs := &c.Tools[j], results[c.Tools[j].ID]
			t.Chars, t.Den, t.Err = rs.chars, rs.den, rs.err && !rs.den
		}
		rt, ok := pricing.For(c.Model)
		c.Priced, c.Cost = ok, rt.Cost(c.In, c.Out, c.CR, c.CW5, c.CW1)
	}
	sort.SliceStable(calls, func(a, b int) bool { return calls[a].TS < calls[b].TS })
	proj := strings.TrimRight(cwd, `/\`)
	proj = proj[strings.LastIndexAny(proj, `/\`)+1:]
	return &Session{
		ID:      pick(sid, strings.TrimSuffix(filepath.Base(path), ".jsonl")),
		Project: pick(proj, filepath.Base(filepath.Dir(path))),
		Branch:  branch, Prompt: prompt, Calls: calls,
	}, nil
}

func pick(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func hasTool(ts []Tool, id string) bool {
	for _, t := range ts {
		if t.ID == id {
			return true
		}
	}
	return false
}

type entry struct {
	mod  time.Time
	size int64
	s    *Session
}

var (
	mu    sync.Mutex
	cache = map[string]entry{}
)

// LoadAll parses every .jsonl under root, in parallel, re-reading only changed files.
// Sessions sharing an id (resumed sessions) resolve in file-path order.
func LoadAll(root string) map[string]*Session {
	var files []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".jsonl") {
			files = append(files, p)
		}
		return nil
	})
	sort.Strings(files)
	found := make([]*Session, len(files))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for i, p := range files {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			st, err := os.Stat(p)
			if err != nil {
				return
			}
			mu.Lock()
			e, ok := cache[p]
			mu.Unlock()
			if !ok || !e.mod.Equal(st.ModTime()) || e.size != st.Size() {
				s, _ := Parse(p)
				e = entry{st.ModTime(), st.Size(), s}
				mu.Lock()
				cache[p] = e
				mu.Unlock()
			}
			found[i] = e.s
		}()
	}
	wg.Wait()
	out := map[string]*Session{}
	for _, s := range found {
		if s != nil && len(s.Calls) > 0 {
			out[s.ID] = s
		}
	}
	return out
}
