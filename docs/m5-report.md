# M5 results (2026-10-17)

Scope: the continuous_load archetype (spec §7) on the Haunted Mansion.

## What was built
- **Shared front of house** (`internal/ride/lines`). Arrival, balking, outdoor lines with reneging, the LL
  merge, the preshow and the combined load line moved out of the coaster. Behavior-preserving: three
  coaster scenario logs are byte-identical before and after.
- **`internal/ride/continuous`.** A conveyor clock runs only while the chain moves, so any stop freezes
  every buggy at once and there is no stacking. One buggy reaches the load zone every `spacing_s`. The head
  party boards; parties never share with strangers, use up to 3 per buggy in the fewest buggies, and fill
  consecutive buggies (5 -> 3 + 2). `board_fail_rate` sends a buggy on empty. Accessibility holds,
  chain_stop, long_downtime and weather all freeze the chain.
- **Timing** (decision 2026-10-17): spacing drives the loop (160 x 3.6 = 576 s); route est_s are stretched
  x 1.218, so board-to-unload is ~531 s (435 s [fan] until spacing is observed).
- **Data** [user]: load area 120 (at normal loads at least one preshow group is ahead of you in the load
  area); no foyer staging; Lightning Lane merges right before the foyer.

## Results against the targets (spec §11)
| Target | Result |
|---|---|
| 3600 / spacing x occupancy | solo guests: 980/hr (1,000 buggies/hr x 0.98 boarding misses) |
| Hard ceiling 3600 / spacing x 3 | 2,940/hr without the stretching room (3,000 x 0.98) |
| Park party mix | occupancy 2.23 per buggy -> 2,198/hr (hand math 2,184) |
| ~2,400/hr listed | **not reached**: needs 2.4 per buggy; the park mix with no stranger sharing gives 2.23 |
| Accessibility holds measurably reduce throughput | 8/hr x 75 s: availability 83.9%, throughput -16.1% (hand math 16.7%) |
| Stops halt everything, no stacking | 10 min hold: no buggy loads, every buggy's ride time shifts by exactly 10 min |
| Contiguous parties | parties of 5 ride 3 + 2 in consecutive buggies |
| LL merge | share = demand / throughput below the cap |
| Determinism, Little's Law | byte-identical; Little within 2% with abandonment |

## Baseline day (`hm-baseline-day`, park mix, all stops on)
1,637 riders/hr, 79.9% availability (accessibility holds 5,420 s, chain stops 1,091 s), standby 16.0 min
mean / 25.4 p95 (Queue-Times target ~26 / 42), 0.95 empty seats per buggy, Little's Law 0.4%.

## Flags
1. **All-trio crowds hit the stretching room first.** 2 rooms x 60 / 165 s = 2,618/hr is below the ride's
   3-per-buggy ceiling of 3,000/hr. With the park mix the ride (2,198/hr) is still the bottleneck.
2. **Accessibility holds are the dominant loss** (~17% of time). Rate 8/hr and 75 s are both [assumed]:
   the highest-value observation for this ride after spacing.
3. **Listed 2,400/hr needs either 2.4 riders per buggy** (strangers sharing, or larger parties) or shorter
   spacing (2.96 s from the route). Occupancy is now an output, so the gap is visible rather than assumed away.
4. The metrics report still prints coaster-only lines (block holds, load idle) for continuous runs; harmless.
