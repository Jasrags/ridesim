package scenario

import (
	"bytes"
	"math"
	"path/filepath"
	"testing"

	"github.com/jasrags/ridesim/internal/config"
	"github.com/jasrags/ridesim/internal/engine"
	"github.com/jasrags/ridesim/internal/guest"
)

func loadHM(t *testing.T) *config.ContinuousFile {
	t.Helper()
	f, err := config.LoadFile(filepath.Clean("../../data/rides/wdw-mk-haunted-mansion.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return f.Continuous
}

func hmRun(cfg *config.ContinuousFile, rate float64, prof guest.Profile) ContinuousRun {
	return ContinuousRun{Ride: cfg, Seed: 1, FixedTimes: true, Length: 8 * engine.Hour, Warmup: engine.Hour,
		Arrivals: guest.Curve{{RatePerHr: rate}}, Guests: prof, NoAccessibilityHolds: true}
}

func computeRun(t *testing.T, r Runner) (*engine.Log, *metricsReport) {
	t.Helper()
	log, err := r.Run()
	if err != nil {
		t.Fatal(err)
	}
	m, err := metricsCompute(log.Events(), r.WarmupDuration())
	if err != nil {
		t.Fatal(err)
	}
	return log, m
}

// 3600 / 3.6 s = 1,000 buggies/hr. Solo guests ride one per buggy; 2% of boardings miss the buggy.
func TestHMCapacitySolo(t *testing.T) {
	_, m := computeRun(t, hmRun(loadHM(t), 4000, guest.Profile{}))
	if want := 1000 * (1 - 0.02); !within(m.RidersPerHr, want, 1.5) {
		t.Errorf("%.0f riders/hr, want ~%.0f", m.RidersPerHr, want)
	}
	if !within(m.Interval.Mean, 3.6, 0.1) {
		t.Errorf("buggy interval %.3f s, want 3.6", m.Interval.Mean)
	}
}

// Ride ceiling: 3 per buggy = 3,000/hr (x 0.98 boarding misses), measured without the stretching room.
// With it, 2 rooms x 60 / (120 + 45 s) = 2,618/hr caps the ride first: the preshow is the bottleneck
// for all-trio crowds.
func TestHMSeatCeilingAndPreshowCap(t *testing.T) {
	trios := guest.Profile{PartySizes: guest.PartySizesFrom(map[int]float64{3: 1})}
	cfg := loadHM(t)
	noPreshow := *cfg
	noPreshow.Preshow = nil
	_, ride := computeRun(t, hmRun(&noPreshow, 6000, trios))
	if want := 3000 * 0.98; !within(ride.RidersPerHr, want, 1.5) {
		t.Errorf("no preshow: %.0f riders/hr, want ~%.0f", ride.RidersPerHr, want)
	}
	_, full := computeRun(t, hmRun(cfg, 6000, trios))
	if want := 2.0 * 60 * 3600 / 165; !within(full.RidersPerHr, want, 2) {
		t.Errorf("with preshow: %.0f riders/hr, want the preshow's %.0f", full.RidersPerHr, want)
	}
}

// Park party mix, up to 3 per buggy, strangers never share: occupancy = E[size] / E[ceil(size/3)].
func TestHMCapacityParkMix(t *testing.T) {
	ds := loadData(t)
	park := ds.Park("synthetic_park_v0")
	var size, buggies float64
	for s, w := range park.Guests.PartySize {
		size += float64(s) * w
		buggies += math.Ceil(float64(s)/3) * w
	}
	want := 1000 * size / buggies * 0.98
	prof := guest.Profile{PartySizes: guest.PartySizesFrom(park.Guests.PartySize)}
	_, m := computeRun(t, hmRun(loadHM(t), 4000, prof))
	t.Logf("occupancy %.2f per buggy -> want %.0f/hr, got %.0f", size/buggies, want, m.RidersPerHr)
	if !within(m.RidersPerHr, want, 2) {
		t.Errorf("%.0f riders/hr, want ~%.0f", m.RidersPerHr, want)
	}
}

// Accessibility holds (8/hr x 75 s, fixed) freeze the chain 600 s per hour: ~16.7% less throughput.
func TestHMAccessibilityHoldsReduceThroughput(t *testing.T) {
	base := hmRun(loadHM(t), 4000, guest.Profile{})
	base.Length = 40 * engine.Hour
	_, off := computeRun(t, base)
	on := base
	on.NoAccessibilityHolds = false
	_, m := computeRun(t, on)
	loss := 1 - m.RidersPerHr/off.RidersPerHr
	t.Logf("availability %.1f%%, throughput loss %.1f%%", m.Ops.Availability*100, loss*100)
	if !within(loss, 600.0/3600, 15) || !within(m.Ops.Availability, 1-600.0/3600, 3) {
		t.Errorf("loss %.3f, availability %.3f; want ~0.167 and ~0.833", loss, m.Ops.Availability)
	}
}

// A party of 5 rides 3 + 2 in consecutive buggies.
func TestHMLargePartiesUseConsecutiveBuggies(t *testing.T) {
	cfg := loadHM(t)
	cfg.LoadZone.BoardFailRate = 0
	fives := guest.Profile{PartySizes: guest.PartySizesFrom(map[int]float64{5: 1})}
	log, _ := computeRun(t, hmRun(cfg, 1000, fives))
	var prev engine.Event
	pairs := 0
	for _, e := range log.Events() {
		if e.Type != engine.EvDispatch || e.N == 0 {
			continue
		}
		if prev.N == 3 {
			if e.N != 2 || (e.Train != prev.Train+1 && !(prev.Train == 160 && e.Train == 1)) {
				t.Fatalf("party of 5 split %d (buggy %d) then %d (buggy %d), want 3 + 2 in consecutive buggies", prev.N, prev.Train, e.N, e.Train)
			}
			pairs++
		}
		prev = e
	}
	if pairs < 100 {
		t.Fatalf("only %d parties of 5 seen", pairs)
	}
}

// A 10 min weather hold freezes the chain: nothing loads, and a buggy loaded before it unloads 10 min
// late. Board to unload is the stretched route: 436 s x 576 / 473 = ~531 s.
func TestHMStopFreezesChainAndRideTime(t *testing.T) {
	cfg := loadHM(t)
	cfg.LoadZone.BoardFailRate = 0
	r := hmRun(cfg, 2000, guest.Profile{})
	hold := Window{From: 2 * engine.Hour, To: 2*engine.Hour + 10*engine.Minute}
	r.Weather = []Window{hold}
	log, _ := computeRun(t, r)
	rideTime := engine.Seconds(436.0 * 576 / 473)
	loaded := map[int]engine.Time{}
	checked := 0
	for _, e := range log.Events() {
		switch e.Type {
		case engine.EvDispatch:
			if e.T > engine.Time(hold.From) && e.T < engine.Time(hold.To) {
				t.Fatalf("buggy dispatched at %v during the hold", e.T)
			}
			if e.N > 0 {
				loaded[e.Train] = e.T
			}
		case engine.EvUnload:
			got := e.T.Sub(loaded[e.Train])
			want := rideTime
			if loaded[e.Train] < engine.Time(hold.From) && e.T > engine.Time(hold.From) {
				want += hold.To - hold.From
			}
			if d := got - want; d < -engine.Second || d > engine.Second {
				t.Fatalf("buggy %d rode %v ms, want %v ms", e.Train, got, want)
			}
			checked++
		}
	}
	if checked < 1000 {
		t.Fatalf("only %d unloads checked", checked)
	}
}

// LL merges right before the foyer [user]: below the cap the share is demand over throughput.
func TestHMLightningLaneMerge(t *testing.T) {
	r := hmRun(loadHM(t), 4000, guest.Profile{})
	r.LLArrivals = guest.Curve{{RatePerHr: 200}}
	_, m := computeRun(t, r)
	if want := 200 / m.RidersPerHr; !within(m.LLShare, want, 5) {
		t.Errorf("LL share %.3f, want %.3f", m.LLShare, want)
	}
}

func TestHMDeterministicAndLittle(t *testing.T) {
	ds := loadData(t)
	prof, err := guestProfile(ds, loadHM(t).Queues, &config.ScenarioGuests{ParkID: "synthetic_park_v0"})
	if err != nil {
		t.Fatal(err)
	}
	r := ContinuousRun{Ride: loadHM(t), Seed: 4, Length: 12 * engine.Hour,
		Arrivals: guest.Curve{{Start: 0, RatePerHr: 2600}, {Start: 2 * engine.Hour, RatePerHr: 900}},
		Guests:   prof, Failures: []string{config.FailChainStop, config.FailLongDowntime}}
	a, m := computeRun(t, r)
	b, _ := computeRun(t, r)
	if !bytes.Equal(encode(t, a), encode(t, b)) {
		t.Fatal("same seed, different logs")
	}
	if m.Unserved > 50 || m.Little.ErrPct > 2 {
		t.Errorf("Little's Law: L %.1f vs lambda*W %.1f (%.1f%%), unserved %d", m.Little.L, m.Little.LambdaW, m.Little.ErrPct, m.Unserved)
	}
}
