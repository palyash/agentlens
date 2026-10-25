package analyze

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/palyash/agentlens/internal/pricing"
	"github.com/palyash/agentlens/internal/transcript"
)

// Fix is one ranked recommendation. Save is an estimate in USD per week.
type Fix struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Evidence []string `json:"evidence"`
	Fix      string   `json:"fix"`
	Save     float64  `json:"save"`
}

type toolEv struct {
	name, fam, arg   string
	tok, carry, save float64
	err, retry       bool
}
type callEv struct {
	ts          string
	miss, light bool
	extra, save float64
}

func unix(ts string) float64 {
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return 0
	}
	return float64(t.UnixNano()) / 1e9
}

func buildEvents(ss map[string]*transcript.Session) (te []toolEv, ce []callEv) {
	sonnet, _ := pricing.For("claude-sonnet-5-5")
	for _, id := range ids(ss) {
		cs := ss[id].Calls
		suf := suffix(cs)
		var prevT float64
		var prevKey [2]string
		prevFail := false
		for ci, c := range cs {
			t0 := unix(c.TS)
			gap := 0.0
			if prevT != 0 {
				gap = t0 - prevT
			}
			prevT = t0
			p, _ := pricing.For(c.Model)
			nr := c.In + c.CW()
			ctx := nr + c.CR
			miss := gap > 300 && ctx > 5000 && float64(c.CR) < 0.5*float64(ctx)
			extra := 0.0
			if miss {
				extra = (float64(c.In)*p[0] + float64(c.CW5)*p[3] + float64(c.CW1)*p[4] - float64(nr)*p[2]) / 1e6
			}
			light := strings.Contains(strings.ToLower(c.Model), "opus") && c.Out < 250 && len(c.Tools) <= 1
			save := 0.0
			if light {
				save = max(c.Cost-sonnet.Cost(c.In, c.Out, c.CR, c.CW5, c.CW1), 0)
			}
			ce = append(ce, callEv{c.TS, miss, light, max(extra, 0), save})
			for _, t := range c.Tools {
				tok := float64(t.Chars) / 4
				key := [2]string{t.Name, t.Arg}
				te = append(te, toolEv{t.Name, t.Fam, t.Arg, tok, tok * suf[ci+1] / 1e6, max(tok-2000, 0) * suf[ci+1] / 1e6,
					t.Err, t.Err && prevFail && prevKey == key})
				prevKey, prevFail = key, t.Err
			}
		}
	}
	return
}

type kv struct {
	k string
	v int
}

func topArgs(es []toolEv, n, minCount int) []kv {
	cnt := map[string]int{}
	for _, e := range es {
		cnt[e.arg]++
	}
	var out []kv
	for k, v := range cnt {
		if k != "" && v >= minCount {
			out = append(out, kv{k, v})
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].v > out[b].v || out[a].v == out[b].v && out[a].k < out[b].k })
	return head(out, n)
}

func kf(n float64) string {
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", n/1000)
	}
	return fmt.Sprintf("%d", int(n))
}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

func sum[T any](xs []T, f func(T) float64) (t float64) {
	for _, x := range xs {
		t += f(x)
	}
	return
}

func filter[T any](xs []T, f func(T) bool) (out []T) {
	for _, x := range xs {
		if f(x) {
			out = append(out, x)
		}
	}
	return
}

// Recommend runs the detectors over all sessions and returns the triggered fixes, biggest saving first.
func Recommend(ss map[string]*transcript.Session) []Fix {
	te, ce := buildEvents(ss)
	var first, last float64
	for _, e := range ce {
		if t := unix(e.ts); t > 0 && (first == 0 || t < first) {
			first = t
		}
		last = max(last, unix(e.ts))
	}
	weeks := max((last-first)/(7*86400), 1)

	bash := filter(te, func(e toolEv) bool { return e.name == "Bash" })
	famToks := map[string][]float64{}
	for _, e := range bash {
		f := e.fam
		if f == "" {
			f = "?"
		}
		famToks[f] = append(famToks[f], e.tok)
	}
	type noisy struct {
		fam   string
		avg   float64
		calls int
	}
	var ns []noisy
	for f, v := range famToks {
		if a := sum(v, func(x float64) float64 { return x }) / float64(len(v)); len(v) >= 5 && a >= 1500 {
			ns = append(ns, noisy{f, a, len(v)})
		}
	}
	sort.Slice(ns, func(a, b int) bool {
		wa, wb := ns[a].avg*float64(ns[a].calls), ns[b].avg*float64(ns[b].calls)
		return wa > wb || wa == wb && ns[a].fam < ns[b].fam
	})
	ns = head(ns, 4)
	big := len(filter(bash, func(e toolEv) bool { return e.tok > 3000 }))
	retries := filter(bash, func(e toolEv) bool { return e.retry })
	setup := filter(bash, func(e toolEv) bool { return e.fam == "export" || e.fam == "source" || e.fam == "eval" })
	misses := filter(ce, func(e callEv) bool { return e.miss })
	lights := filter(ce, func(e callEv) bool { return e.light })

	var out []Fix
	if big >= 5 {
		ev := []string{fmt.Sprintf("%d Bash results returned over ~3k tokens each. Every one is re-read on all later requests.", big)}
		fix := "## Command output\n- Pipe long command output through `tail -n 40` or `head -n 40`; never print full logs.\n" +
			"- Run a targeted test first (for example `go test ./pkg -run TestName`), the whole suite only when asked.\n" +
			"- Prefer `git diff --stat` and `git log --oneline -n 10`."
		var names []string
		for _, n := range ns {
			ev = append(ev, fmt.Sprintf("%s: ~%s tokens per call over %d calls.", n.fam, kf(n.avg), n.calls))
			names = append(names, n.fam)
		}
		if len(names) > 0 {
			fix += "\n- Noisy in my sessions: " + strings.Join(names, ", ") + "."
		}
		out = append(out, Fix{"cap-output", "Cap noisy Bash output", ev, fix, sum(bash, func(e toolEv) float64 { return e.save })})
	}
	if len(retries) >= 3 {
		ev := []string{fmt.Sprintf("%d Bash calls re-ran the exact command that had just failed.", len(retries))}
		for _, a := range topArgs(filter(bash, func(e toolEv) bool { return e.err }), 3, 2) {
			ev = append(ev, fmt.Sprintf("'%s' failed %d times.", clip(a.k, 60), a.v))
		}
		out = append(out, Fix{"stop-retries", "Stop retrying failed commands unchanged", ev,
			"## Failures\n- When a command fails, read the error and change something before running it again.\n- Never re-run an identical failing command more than once.",
			sum(retries, func(e toolEv) float64 { return e.carry })})
	}
	if len(setup) >= 3 {
		ev := []string{fmt.Sprintf("%d Bash calls were export/source/eval setup.", len(setup))}
		for _, a := range topArgs(setup, 3, 2) {
			ev = append(ev, fmt.Sprintf("'%s' ran %d times.", clip(a.k, 60), a.v))
		}
		out = append(out, Fix{"repeat-setup", "Stop re-running environment setup", ev,
			"## Environment\n- My shell environment (PATH and variables) is already set up. Do not export or source it inside commands.\n\n" +
				"If something is really missing, add it once to your shell profile or the `env` section of .claude/settings.json.",
			sum(setup, func(e toolEv) float64 { return e.carry })})
	}
	if len(misses) >= 5 {
		out = append(out, Fix{"idle-gaps", "Avoid cache rebuilds after idle gaps",
			[]string{fmt.Sprintf("%d requests came after a pause of 5+ minutes and re-sent their context at full price.", len(misses))},
			"Habit, not a config change:\n- Before a long break, finish the task or run /clear.\n- After a break, start with a short summary instead of resuming a huge session.",
			sum(misses, func(e callEv) float64 { return e.extra })})
	}
	if len(lights) >= 10 {
		out = append(out, Fix{"light-on-opus", "Use a cheaper model for light requests",
			[]string{fmt.Sprintf("%d short requests (under 250 output tokens, at most one tool call) ran on Opus-class models.", len(lights))},
			"Habit, not a config change:\n- Use /model to switch to a cheaper model for exploration, small edits, and simple questions.\n" +
				"- Keep the larger model for design and hard debugging.\nQuality is your call: compare results on a few tasks first.",
			sum(lights, func(e callEv) float64 { return e.save })})
	}
	for i := range out {
		out[i].Save /= weeks
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].Save > out[b].Save })
	if out == nil {
		out = []Fix{}
	}
	return out
}
