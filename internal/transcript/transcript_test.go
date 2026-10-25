package transcript

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

const sample = `{"type":"user","sessionId":"s1","cwd":"/work/proj","message":{"content":"hello"}}
{"type":"assistant","timestamp":"2026-01-01T00:00:01Z","message":{"id":"m1","model":"claude-sonnet-5-5","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"cd /x && go test ./..."}}],"usage":{"input_tokens":1000000,"output_tokens":10}}}
{"type":"assistant","timestamp":"2026-01-01T00:00:02Z","message":{"id":"m1","model":"claude-sonnet-5-5","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"cd /x && go test ./..."}}],"usage":{"input_tokens":1000000,"output_tokens":30}}}
{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","is_error":true,"content":"FAIL"}]}}
`

func TestParseMergesStreamedMessagesAndPrices(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(p, []byte(sample), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Parse(p)
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != "s1" || s.Project != "proj" || len(s.Calls) != 1 {
		t.Fatalf("unexpected session: %+v", s)
	}
	c := s.Calls[0]
	if c.Out != 30 || len(c.Tools) != 1 {
		t.Fatalf("streamed duplicates not merged: %+v", c)
	}
	if want := 2 + 30*10/1e6; math.Abs(c.Cost-want) > 1e-9 {
		t.Fatalf("cost = %v, want %v", c.Cost, want)
	}
	if tl := c.Tools[0]; !tl.Err || tl.Arg != "go test ./..." || tl.Fam != "go test" {
		t.Fatalf("tool not parsed as expected: %+v", tl)
	}
}
