// Package pricing holds per-million-token list prices used to turn token counts
// into API-equivalent cost estimates. Verify the table against current pricing.
package pricing

import "strings"

// Rates are USD per 1M tokens: input, output, cache read, cache write 5m, cache write 1h.
type Rates [5]float64

// Ordered most specific first: the first key contained in the model name wins.
var table = []struct {
	key string
	r   Rates
}{
	{"haiku-5", Rates{0.10, 0.50, 0.01, 0.125, 0.20}},
	{"sonnet-5", Rates{2, 10, 0.10, 2.5, 4}},
	{"opus-5-5", Rates{4, 20, 0.20, 5, 8}},
	{"opus-5", Rates{5, 25, 0.50, 6.25, 10}},
	{"haiku-4", Rates{1, 5, 0.10, 1.25, 2}},
	{"sonnet-4", Rates{3, 15, 0.30, 3.75, 6}},
	{"opus-4-5", Rates{5, 25, 0.50, 6.25, 10}},
	{"opus-4", Rates{15, 75, 1.5, 18.75, 30}},
}

// For returns the rates for a model name; ok is false when the model is unpriced.
func For(model string) (Rates, bool) {
	m := strings.ReplaceAll(strings.ToLower(model), ".", "-")
	for _, t := range table {
		if strings.Contains(m, t.key) {
			return t.r, true
		}
	}
	return Rates{}, false
}

// Cost prices one request.
func (r Rates) Cost(in, out, cacheRead, cw5, cw1 int) float64 {
	return (float64(in)*r[0] + float64(out)*r[1] + float64(cacheRead)*r[2] + float64(cw5)*r[3] + float64(cw1)*r[4]) / 1e6
}
