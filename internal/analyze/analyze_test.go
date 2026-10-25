package analyze

import (
	"encoding/json"
	"math"
	"os"
	"testing"

	"github.com/palyash/agentlens/internal/transcript"
)

// golden.json was produced by the original Python prototype on the same fixtures,
// so this test pins the Go port to the reference implementation.
func TestMatchesReferenceImplementation(t *testing.T) {
	var want struct {
		Kpi      map[string]float64 `json:"kpi"`
		Bash     map[string]float64 `json:"bash"`
		Tools    int                `json:"tools"`
		Fixes    []string           `json:"fixes"`
		Projects []string           `json:"projects"`
	}
	b, err := os.ReadFile("testdata/golden.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}
	ss := transcript.LoadAll("../demo/data")
	got := Aggregate(ss, 0, "")
	kpi := got["kpi"].(map[string]float64)
	for k, w := range want.Kpi {
		if math.Abs(kpi[k]-w) > 1e-6*math.Max(1, math.Abs(w)) {
			t.Errorf("kpi.%s = %v, want %v", k, kpi[k], w)
		}
	}
	bs := got["bash"].(Report)["sum"].(map[string]float64)
	for k, w := range want.Bash {
		if bs[k] != w {
			t.Errorf("bash.%s = %v, want %v", k, bs[k], w)
		}
	}
	if n := len(got["tools"].([]toolStat)); n != want.Tools {
		t.Errorf("tools = %d, want %d", n, want.Tools)
	}
	for i, p := range got["projects"].([]nv) {
		if p.N != want.Projects[i] {
			t.Errorf("projects[%d] = %s, want %s", i, p.N, want.Projects[i])
		}
	}
	var ids []string
	for _, f := range Recommend(ss) {
		ids = append(ids, f.ID)
	}
	if len(ids) != len(want.Fixes) || (len(ids) > 0 && ids[0] != want.Fixes[0]) {
		t.Errorf("fixes = %v, want %v", ids, want.Fixes)
	}
}

func TestRetriesAndCarry(t *testing.T) {
	calls := []transcript.Call{
		{TS: "2026-01-01T00:00:01Z", Model: "claude-sonnet-5-5", Priced: true,
			Tools: []transcript.Tool{{ID: "a", Name: "Bash", Arg: "make", Fam: "make", Chars: 80000, Err: true}}},
		{TS: "2026-01-01T00:00:02Z", Model: "claude-sonnet-5-5", Priced: true,
			Tools: []transcript.Tool{{ID: "b", Name: "Bash", Arg: "make", Fam: "make", Chars: 400, Err: true}}},
		{TS: "2026-01-01T00:00:03Z", Model: "claude-sonnet-5-5", Priced: true},
	}
	rep := Aggregate(map[string]*transcript.Session{"s": {ID: "s", Project: "p", Calls: calls}}, 0, "")
	bs := rep["bash"].(Report)["sum"].(map[string]float64)
	if bs["retries"] != 1 || bs["calls"] != 2 || bs["err"] != 2 {
		t.Fatalf("bash sum = %v", bs)
	}
	// first result: 20k tokens re-read by 2 later requests at $0.10/M
	if want := 20000 * 2 * 0.10 / 1e6; bs["carry"] < want {
		t.Fatalf("carry %v below expected %v", bs["carry"], want)
	}
}
