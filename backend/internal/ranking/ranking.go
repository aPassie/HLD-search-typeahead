// Package ranking holds the recency-scoring math: exponential time-decay with a
// half-life. The functions are pure; reads compute a query's effective recency
// without mutating state. Only the batch writer updates the stored value.
package ranking

import (
	"math"
	"time"
)

// Params captures the decay rate and the score weights.
type Params struct {
	Lambda float64 // decay constant per second = ln2 / half-life
	Alpha  float64 // weight of recency in the combined score
	Weight float64 // recency added per search
}

// New builds Params from a half-life. λ = ln(2)/half-life, so a value halves
// after one half-life.
func New(halfLife time.Duration, alpha, weight float64) Params {
	hl := halfLife.Seconds()
	if hl <= 0 {
		hl = 1800
	}
	return Params{Lambda: math.Ln2 / hl, Alpha: alpha, Weight: weight}
}

// Effective decays a stored recency value to `now` (read-only; ts/now in unix seconds).
func (p Params) Effective(value float64, ts, now int64) float64 {
	if value <= 0 {
		return 0
	}
	dt := now - ts
	if dt < 0 {
		dt = 0 // guard against clock skew
	}
	return value * math.Exp(-p.Lambda*float64(dt))
}

// Bump applies d searches at time `now`: decay the old value to now, then add
// d·weight. Returns the new (value, ts).
func (p Params) Bump(value float64, ts, d, now int64) (float64, int64) {
	return p.Effective(value, ts, now) + float64(d)*p.Weight, now
}

// Score combines all-time popularity with current recency. log() compresses the
// large count range so the recency term still affects the ordering.
func (p Params) Score(count int64, effRecent float64) float64 {
	return math.Log(1+float64(count)) + p.Alpha*effRecent
}

// Key is the time-invariant ordering key s = ln(value) + λ·ts. Pairwise recency
// order doesn't change as time passes, so trending can evict by s without
// re-decaying every entry. s increases on every search.
func (p Params) Key(value float64, ts int64) float64 {
	if value <= 0 {
		return math.Inf(-1)
	}
	return math.Log(value) + p.Lambda*float64(ts)
}
