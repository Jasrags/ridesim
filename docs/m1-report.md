# M0/M1 results (2026-10-17)

**Status:** the Everest retune below was approved and applied to the data file the same day (see
`docs/tuning-log.md`). "Data as authored" columns describe the pre-retune values, now kept in test code as
`preTuneValues` for the bottleneck regression.

All numbers are reproducible with `go test ./...` (acceptance tests in `internal/scenario/everest_test.go`)
or `ridesim run`. "Saturated" means 6,000 guests/hr of standby demand, so every train leaves full.

## Results against the targets

| Target (spec §11) | Data as authored (pre-retune) | With retune (now in the data file) |
|---|---|---|
| 5 trains: dispatch ~54 s | **71.0 s (miss)** | 54.0 s |
| 5 trains: ~2,267 riders/hr | **1,724/hr (miss)** | 2,264/hr |
| 4 trains: gap where the fifth would be | **no gap**: 71.0 s, same as 5 (station idle 22.5% vs 0%) | 54, 54, 54, 100 s pattern, mean 65.6 s, 1,870/hr |
| Same seed, byte-identical log | pass | pass |
| Little's Law at steady state | pass: L 11.36 vs λW 11.36 at 97% load | n/a |
| Brake holds at shadow room, pre-unload, peak; rare at yeti | shadow and peak every pass; pre-unload ~1%; yeti never | stochastic: shadow ~100%, peak ~45%, pre-unload ~25%, yeti 1–2 in ~490 |
| Listed 2,050/hr after real-world losses | n/a | stochastic times alone give 2,061–2,087/hr (failures still off) |

### Why the authored data misses (explained before any tuning)
Clearance rules are applied literally. `dispatch_out_of_mountain` lets a train leave SHADOW_BRAKE only when
MOUNTAIN_EXIT **and** HELIX_TO_BRAKES are empty. That makes one 45 + 24 = 69 s region with a train always
waiting at its entry brake, which then pays the 2 s release delay: a 71 s headway, giving 3600/71 x 34 = 1,724/hr.
The station alone would allow 44 + 8 + 3 = 55 s. A second region also exceeds 54 s:
LIFT + PEAK_BRAKE + switch realign = 45 + 6 + 8 = 59 s. The model matches this hand math exactly
(`TestEverestAsAuthoredMatchesClearanceHandMath`).

Four trains don't show a gap because 4 x 71 = 284 s is longer than the 263 s unimpeded lap, so the
bottleneck still sets the pace. The fifth train's slot shows up only as station idle time.

### Retune (approved and applied 2026-10-17; [assumed] values only, sum stays 170 s)
| Field | Before | After | Why |
|---|---|---|---|
| LIFT.transit_s.mean | 45 | 40 | LIFT + PEAK + realign = 54 s |
| SHADOW_BRAKE.transit_s.mean | 12 | 20 | takes up the time removed elsewhere |
| MOUNTAIN_EXIT.transit_s.mean | 45 | 32 | MOUNTAIN_EXIT + HELIX = 52 s < 54 |
| HELIX_TO_BRAKES.transit_s.mean | 24 | 20 | same region |
| YETI_BRAKE.transit_s.mean | 4 | 6 | keeps the sum at 170 |
| PRE_UNLOAD_HOLD.transit_s.mean | 14 | 26 | keeps the sum at 170 (yeti scene + final stretch) |
| station.load_s.mean | 44 | 43 | station cycle 43 + 8 + 3 = 54 s |

PEAK_BRAKE (6 s) and REVERSE (20 s) are `[fan]` and unchanged. Applied to the YAML and logged in
`docs/tuning-log.md`; the acceptance tests now run against the file.

## Decisions made during M1
- **hold_until_clear yields when it would deadlock** (your call). With 5 trains and literal clearance, the
  shadow-room hold forms a circular wait: shadow train waits for UNLOAD->LOAD, which needs LOAD to dispatch,
  which needs PEAK_BRAKE empty, and the PEAK train waits for SHADOW_BRAKE. The real ride runs 5 trains
  ([wiki]), so one [fan] rule is stricter than reality. The hold now also releases when another held train
  needs SHADOW_BRAKE empty, logged as `hold_yield`.
  As authored, it yields on ~100% of passes, so the rule never effectively holds. With the retune it
  yields on ~40% of passes.
- Station handoff UNLOAD->LOAD takes 0 s. `load_clear_to_advance` adds no gate beyond UNLOAD being empty.
- `max_wait_after_ready_s` runs after `load_s` and before the restraint check; guests keep boarding during it.
- A train may not enter a block whose switch is still realigning (Everest's switch 2), in addition to
  the explicit `switch_aligned` condition on LOAD dispatch.
- Distributions are normal, truncated at 0. `--fixed-times` uses the means.
- Station times draw from the `station_times` stream; transit, release and realign times from `track_times`.

## Bug found and fixed
A train that became held later in the same evaluation pass than the shadow-room train didn't trigger the
yield. That deadlocked 5-train runs (LIFT +10%, and every stochastic run with the retune). The sensitivity
sweep caught it. There are now regression tests, and both fail without the fix.

## [assumed] values that matter most (fixed times, 5 trains, +10% one at a time)
| Value | As authored | With retune |
|---|---|---|
| MOUNTAIN_EXIT.transit | -5.9% throughput | -5.4% |
| HELIX_TO_BRAKES.transit | -3.2% | -3.6% |
| LIFT.transit | **deadlock** (fixed above) | -17.1% (54 -> 65 s: LIFT region becomes the bottleneck) |
| load_s | 0 | -11.4% |
| restraint_check_s | 0 | -3.6% |
| switch_realign_s | 0 | -3.3% |
| dispatch_clearance_s | 0 | -2.1% |
| release_delay_s | -0.4% | -0.3% |
| SHADOW, YETI, PRE_UNLOAD transit; unload_s | 0 | 0 |

As authored, only the mountain-exit region matters. With the retune, the result sits on a knife edge:
the station cycle, the LIFT region and the mountain-exit region are all within 2 s of 54. That's what
you'd expect of a well-engineered ride, but it means observed per-section times (spec §13 gap #2) are the
most valuable data to collect next.

## Data and spec contradictions flagged
1. Everest clearance regions vs the 54 s target (above): data contradicts the spec's hand math.
2. Everest hold_until_clear + clearance rules: circular wait with 5 trains (above).
3. Spec §3 says filename = ride id "with dots replaced by dashes"; every file also replaces underscores.
   The check follows the files.
4. Rock 'n' Roller: load block 40 + 8 + 3 = 51 s exceeds its 48 s target. It will be station-bound at
   ~1,694/hr in M3 unless its [assumed] station times change. The 82 vs 70 s duration conflict stays on
   Wikipedia's 82, and RCDB's 70 would fail the block-sum invariant.
5. Haunted Mansion: 160 x 3.6 = 576 s vs a 473 s route (21.8% off, warn). The route implies 2.96 s spacing,
   which needs about 2.0 riders per buggy to reach 2,400/hr.
6. Haunted Mansion `loop_time_s` invariant: loop time is derived from count x spacing, and its reference
   (`total_experience_s`) includes the stretching room, which is off the loop. Kept as a warn; it passes (6% off).
7. Files said Lightning Lane is "milestone M5"; spec says M4. Comments corrected.
8. Stale header versions (Everest "v0.1", Rock 'n' Roller "v0.2 schema", Haunted Mansion "v0.3") corrected in the 1.0 migration.
9. Spec's `Ride.PostedWait() time.Duration` became `engine.Duration` (int64 ms) so the sim uses one time type.

## Not done / out of scope
Failures, cascade stops, barn swaps, posted-wait variants (M2); parties > 1, single rider, preshow (M3);
Lightning Lane (M4); continuous_load (M5). The `Ride` interface, `guest.Lane` and `queue.FIFO` are in place
for them.
