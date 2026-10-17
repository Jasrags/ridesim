// Package engine is the deterministic discrete-event core: simulated clock, (time, seq) scheduler,
// named RNG streams, and the append-only event log. It knows nothing about rides.
package engine

import (
	"fmt"
	"math"
)

// Time is simulated time in milliseconds since the start of the run.
type Time int64

// Duration is a span of simulated time in milliseconds.
type Duration int64

const (
	Millisecond Duration = 1
	Second               = 1000 * Millisecond
	Minute               = 60 * Second
	Hour                 = 60 * Minute
)

// Seconds converts float seconds (as written in the YAML) to a Duration, rounding to the nearest ms.
func Seconds(s float64) Duration { return Duration(math.Round(s * 1000)) }

// Seconds returns d in float seconds.
func (d Duration) Seconds() float64 { return float64(d) / 1000 }

// Add returns t+d.
func (t Time) Add(d Duration) Time { return t + Time(d) }

// Sub returns t-u.
func (t Time) Sub(u Time) Duration { return Duration(t - u) }

// Seconds returns t in float seconds since start.
func (t Time) Seconds() float64 { return float64(t) / 1000 }

func (t Time) String() string {
	ms := int64(t)
	return fmt.Sprintf("%02d:%02d:%02d.%03d", ms/3600000, ms/60000%60, ms/1000%60, ms%1000)
}
