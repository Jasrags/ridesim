package engine

import (
	"hash/fnv"
	"math/rand/v2"
)

// Named RNG streams. Each is seeded from the run seed and a hash of its name only, so adding a stream
// or drawing more from one never shifts another.
const (
	StreamArrivals     = "arrivals"
	StreamFailures     = "failures"
	StreamGuestChoice  = "guest_choice"
	StreamStationTimes = "station_times"
	StreamTrackTimes   = "track_times"
	StreamOps          = "ops"
	StreamLLArrivals   = "ll_arrivals"
	StreamLLChoice     = "ll_choice"
)

type rngSet struct {
	seed    uint64
	streams map[string]*rand.Rand // lookup by name only; never iterated
}

func (r *rngSet) stream(name string) *rand.Rand {
	if s, ok := r.streams[name]; ok {
		return s
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(name))
	s := rand.New(rand.NewPCG(r.seed, h.Sum64()))
	r.streams[name] = s
	return s
}
