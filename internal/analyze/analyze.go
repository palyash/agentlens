// Package analyze turns sessions into the report the dashboard renders.
// Per-tool token figures are estimates: result size / 4. "Carry" is the estimated cost of
// keeping a tool result in context: tokens x later requests x cache-read price.
package analyze

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/palyash/agentlens/internal/pricing"
	"github.com/palyash/agentlens/internal/transcript"
)

type Report map[string]any

type Row struct {
	ID      string  `json:"id"`
	Project string  `json:"project"`
	Branch  string  `json:"branch"`
	Prompt  string  `json:"prompt"`
	Start   string  `json:"start"`
	End     string  `json:"end"`
	Reqs    int     `json:"reqs"`
	Tools   int     `json:"tools"`
	Tokens  int     `json:"tokens"`
	Cache   int     `json:"cache"`
	Cost    float64 `json:"cost"`
}

type nv struct {
	N string  `json:"n"`
	V float64 `json:"v"`
}
type toolStat struct {
	N     string  `json:"n"`
	Calls int     `json:"calls"`
	Chars int     `json:"chars"`
	Err   int     `json:"err"`
	Den   int     `json:"den"`
	Carry float64 `json:"carry"`
}
type bashDay struct {
	D     string  `json:"d"`
	Calls int     `json:"calls"`
	Tok   float64 `json:"tok"`
	Err   int     `json:"err"`
}
type famStat struct {
	N     string  `json:"n"`
	Calls int     `json:"calls"`
	Tok   float64 `json:"tok"`
	Err   int     `json:"err"`
	Carry float64 `json:"carry"`
}
type bigOut struct {
	A     string  `json:"a"`
	Tok   float64 `json:"tok"`
	Carry float64 `json:"carry"`
	Err   bool    `json:"err"`
}
type tv struct {
	Tok   float64 `json:"tok"`
	Carry float64 `json:"carry"`
}
type dayTools struct {
	D string         `json:"d"`
	T map[string]*tv `json:"t"`
}
type failed struct {
	A string `json:"a"`
	N string `json:"n"`
	V int    `json:"v"`
}

// ToolLabel groups MCP tools by server: mcp__jira__create -> "jira (MCP)".
func ToolLabel(n string) string {
	if p := strings.Split(n, "__"); strings.HasPrefix(n, "mcp__") && len(p) > 1 {
		return p[1] + " (MCP)"
	}
	return n
}

func cachePct(i, cr, cw int) int {
	if t := i + cr + cw; t > 0 {
		return int(math.Round(100 * float64(cr) / float64(t)))
	}
	return 0
}

func ids(ss map[string]*transcript.Session) []string {
	out := make([]string, 0, len(ss))
	for id := range ss {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// suffix[j] is the summed cache-read price of requests j.. : what one token pays to be re-read.
func suffix(cs []transcript.Call) []float64 {
	s := make([]float64, len(cs)+1)
	for j := len(cs) - 1; j >= 0; j-- {
		r, _ := pricing.For(cs[j].Model)
		s[j] = s[j+1] + r[2]
	}
	return s
}

func day(ts string) string {
	if len(ts) >= 10 {
		return ts[:10]
	}
	return ts
}

func sortNV(m map[string]float64) []nv {
	out := make([]nv, 0, len(m))
	for k, v := range m {
		out = append(out, nv{k, v})
	}
	sort.Slice(out, func(a, b int) bool { return out[a].V > out[b].V || out[a].V == out[b].V && out[a].N < out[b].N })
	return out
}

// Aggregate builds the dashboard report for the last `days` days (0 = all time) and an
// optional project filter.
func Aggregate(ss map[string]*transcript.Session, days int, project string) Report {
	cutoff := ""
	if days > 0 {
		cutoff = time.Now().UTC().AddDate(0, 0, -days).Format("2006-01-02T15:04:05.999999")
	}
	var (
		daily, tday          = map[string]map[string]float64{}, map[string]map[string]*tv{}
		projs, models, kinds = map[string]float64{}, map[string]float64{}, map[string]float64{}
		tools                = map[string]*toolStat{}
		bday                 = map[string]*bashDay{}
		fams                 = map[string]*famStat{}
		fails                = map[[2]string]int{}
		big                  = map[string]int{}
		unpriced, allp       = map[string]bool{}, map[string]bool{}
		K, bs                = map[string]float64{}, map[string]float64{}
		rows, insights, top  = []Row{}, []string{}, []bigOut{}
	)
	for _, id := range ids(ss) {
		s := ss[id]
		allp[s.Project] = true
		if project != "" && s.Project != project {
			continue
		}
		var cs []transcript.Call
		for _, c := range s.Calls {
			if c.TS >= cutoff {
				cs = append(cs, c)
			}
		}
		if len(cs) == 0 {
			continue
		}
		suf := suffix(cs)
		var sc struct {
			i, o, cr, cw, tools int
			cost                float64
		}
		var prev [2]string
		hasPrev := false
		for ci, c := range cs {
			p, priced := pricing.For(c.Model)
			co := c.Cost
			if !priced {
				co = 0
				unpriced[c.Model] = true
			}
			kinds["Fresh input"] += float64(c.In) * p[0] / 1e6
			kinds["Output"] += float64(c.Out) * p[1] / 1e6
			kinds["Cache read"] += float64(c.CR) * p[2] / 1e6
			kinds["Cache write"] += (float64(c.CW5)*p[3] + float64(c.CW1)*p[4]) / 1e6
			m, d := c.Model, day(c.TS)
			if m == "" {
				m = "?"
			}
			if daily[d] == nil {
				daily[d] = map[string]float64{}
			}
			daily[d][m] += co
			models[m] += co
			K["cost"] += co
			K["i"] += float64(c.In)
			K["o"] += float64(c.Out)
			K["cr"] += float64(c.CR)
			K["cw"] += float64(c.CW())
			K["think"] += float64(c.Think) / 4
			K["calls"]++
			sc.cost += co
			sc.i, sc.o, sc.cr, sc.cw, sc.tools = sc.i+c.In, sc.o+c.Out, sc.cr+c.CR, sc.cw+c.CW(), sc.tools+len(c.Tools)
			for _, t := range c.Tools {
				lab, tok := ToolLabel(t.Name), float64(t.Chars)/4
				carry := tok * suf[ci+1] / 1e6
				x := tools[lab]
				if x == nil {
					x = &toolStat{N: lab}
					tools[lab] = x
				}
				x.Calls++
				x.Chars += t.Chars
				x.Carry += carry
				if t.Err {
					x.Err++
				}
				if t.Den {
					x.Den++
				}
				if tday[d] == nil {
					tday[d] = map[string]*tv{}
				}
				if tday[d][lab] == nil {
					tday[d][lab] = &tv{}
				}
				tday[d][lab].Tok += tok
				tday[d][lab].Carry += carry
				if tok > 10000 {
					big[lab]++
				}
				key := [2]string{t.Name, t.Arg}
				if t.Err {
					fails[key]++
					if hasPrev && prev == key {
						bs["retries"]++
					}
				}
				prev, hasPrev = key, t.Err
				if t.Name != "Bash" {
					continue
				}
				bd, fm := bday[d], fams[t.Fam]
				if bd == nil {
					bd = &bashDay{D: d}
					bday[d] = bd
				}
				if fm == nil {
					fm = &famStat{N: t.Fam}
					fams[t.Fam] = fm
				}
				bd.Calls++
				bd.Tok += tok
				fm.Calls++
				fm.Tok += tok
				fm.Carry += carry
				bs["calls"]++
				bs["carry"] += carry
				bs["save"] += math.Max(tok-2000, 0) * suf[ci+1] / 1e6
				if t.Err {
					bd.Err++
					fm.Err++
					bs["err"]++
				}
				top = append(top, bigOut{t.Arg, tok, carry, t.Err})
			}
		}
		projs[s.Project] += sc.cost
		cp := cachePct(sc.i, sc.cr, sc.cw)
		rows = append(rows, Row{s.ID, s.Project, s.Branch, s.Prompt, cs[0].TS, cs[len(cs)-1].TS, len(cs), sc.tools,
			sc.i + sc.o + sc.cr + sc.cw, cp, sc.cost})
		if cp < 70 && sc.i+sc.cr+sc.cw > 30000 {
			insights = append(insights, fmt.Sprintf("Low cache hit (%d%%) in session %s (%s).", cp, short(s.ID), s.Project))
		}
	}
	bigNames := make([]string, 0, len(big))
	for n := range big {
		bigNames = append(bigNames, n)
	}
	sort.Strings(bigNames)
	for _, n := range bigNames {
		insights = append(insights, fmt.Sprintf("%s returned over ~10k tokens %d time(s). Large outputs are re-read on every later request.", n, big[n]))
	}
	if K["o"] > 0 {
		insights = append(insights, fmt.Sprintf("Thinking is about %d%% of output tokens (estimated from text length).", int(math.Round(100*K["think"]/K["o"]))))
	}
	if len(unpriced) > 0 {
		names := []string{}
		for n := range unpriced {
			names = append(names, n)
		}
		sort.Strings(names)
		insights = append(insights, fmt.Sprintf("No price for: %s. Add it to the pricing table; its cost shows as $0.", strings.Join(names, ", ")))
	}
	if bs["retries"] > 0 {
		insights = append(insights, fmt.Sprintf("%d calls retried the exact command that had just failed.", int(bs["retries"])))
	}
	K["sessions"] = float64(len(rows))
	K["cache"] = float64(cachePct(int(K["i"]), int(K["cr"]), int(K["cw"])))
	sort.Slice(rows, func(a, b int) bool { return rows[a].Start > rows[b].Start })

	dailyOut := make([]Report, 0, len(daily))
	for _, d := range sortedKeys(daily) {
		dailyOut = append(dailyOut, Report{"d": d, "m": daily[d]})
	}
	tdayOut := make([]dayTools, 0, len(tday))
	for _, d := range sortedKeys(tday) {
		tdayOut = append(tdayOut, dayTools{d, tday[d]})
	}
	toolList := make([]toolStat, 0, len(tools))
	for _, t := range tools {
		toolList = append(toolList, *t)
	}
	sort.Slice(toolList, func(a, b int) bool {
		return toolList[a].Carry > toolList[b].Carry || toolList[a].Carry == toolList[b].Carry && toolList[a].N < toolList[b].N
	})
	famList := make([]famStat, 0, len(fams))
	for _, f := range fams {
		famList = append(famList, *f)
	}
	sort.Slice(famList, func(a, b int) bool {
		return famList[a].Carry > famList[b].Carry || famList[a].Carry == famList[b].Carry && famList[a].N < famList[b].N
	})
	days_ := make([]bashDay, 0, len(bday))
	for _, d := range sortedKeys(bday) {
		days_ = append(days_, *bday[d])
	}
	sort.SliceStable(top, func(a, b int) bool { return top[a].Carry > top[b].Carry })
	failList := make([]failed, 0, len(fails))
	for k, v := range fails {
		failList = append(failList, failed{k[1], k[0], v})
	}
	sort.Slice(failList, func(a, b int) bool {
		return failList[a].V > failList[b].V || failList[a].V == failList[b].V && failList[a].A < failList[b].A
	})
	return Report{
		"kpi": K, "daily": dailyOut, "projects": sortNV(projs), "models": sortNV(models), "kinds": kinds,
		"sessions": rows, "tools": toolList, "tday": tdayOut, "insights": insights,
		"bash":        Report{"sum": bs, "days": days_, "fams": head(famList, 12), "top": head(top, 10)},
		"fails":       head(failList, 8),
		"projectList": sortedKeys(allp),
	}
}

func short(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func head[T any](s []T, n int) []T {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

type detailTool struct {
	N   string `json:"n"`
	Arg string `json:"arg"`
	Tok int    `json:"tok"`
}
type detailCall struct {
	TS    string       `json:"ts"`
	Model string       `json:"model"`
	I     int          `json:"i"`
	O     int          `json:"o"`
	CR    int          `json:"cr"`
	CW    int          `json:"cw"`
	Cost  *float64     `json:"cost"`
	Think int          `json:"think"`
	Tools []detailTool `json:"tools"`
}

// Detail returns the request-by-request view of one session.
func Detail(ss map[string]*transcript.Session, id string) Report {
	s := ss[id]
	if s == nil {
		return Report{}
	}
	calls := make([]detailCall, 0, len(s.Calls))
	for _, c := range s.Calls {
		d := detailCall{TS: c.TS, Model: c.Model, I: c.In, O: c.Out, CR: c.CR, CW: c.CW(), Think: int(math.Round(float64(c.Think) / 4)), Tools: []detailTool{}}
		if c.Priced {
			cost := c.Cost
			d.Cost = &cost
		}
		for _, t := range c.Tools {
			d.Tools = append(d.Tools, detailTool{ToolLabel(t.Name), t.Arg, int(math.Round(float64(t.Chars) / 4))})
		}
		calls = append(calls, d)
	}
	return Report{"id": s.ID, "project": s.Project, "branch": s.Branch, "prompt": s.Prompt, "calls": calls}
}
