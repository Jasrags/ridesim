# M4 results (2026-10-17)

Scope: Lightning Lane (LL) merge phases and the realized LL share (spec §8, §11). Also a single-rider
open/close lever ([user]: the line is not always available).

## What was built
- **LL line** per ride. LL guests hold a return time, so they never balk or renege. Arrivals come from
  a scenario `lightning_lane.arrivals` curve in guests/hr, on their own RNG streams. Smooth Poisson for
  now; return-window clustering comes with the park model (M7).
- **Merge** (`queue.Merge`): a credit counter in the configured unit (guests). Each standby unit admitted
  allows `ll_per_standby` LL units; credit accrues only while LL guests are waiting. An empty line hands
  the merge to the other (`pull_other`).
  - Phases follow LL queue length with hysteresis: normal 2:1 below 60, elevated 4:1 below 200,
    recovery 10:1. A phase steps down only once the queue is 20 below its threshold.
- **Merge points**: Rock 'n' Roller in the preshow hallway, so LL guests ride through the show and
  the combined post-show line. Everest on the load line (station queue entrance).
- **Standby posted wait** = standby-side guests / standby riders per second (15 min window).
- **Metrics**: realized LL share, wait by lane (mean / p95), time in each merge phase.
- **Single-rider line** open/close windows in scenarios. While closed, single riders queue standby; those
  already in line stay.

## Results against the targets
| Test | Result |
|---|---|
| Phase ratios honored while both lines wait (2:1, 4:1, 10:1), both units, hysteresis | pass |
| Below the cap: LL share = LL demand / throughput, no capacity cost | Everest 400 LL/hr: share = 400 / 2,264; throughput unchanged |
| LL saturated: recovery caps LL at 10/11 of riders | 90.9% |
| Downtime backlog (30 min hold, ~196 LL guests) | phased merge clears it in 456 s vs 564 s for flat 2:1 |
| Rock 'n' Roller hallway merge | share = demand / throughput; 1,800/hr kept |
| **Realized LL share bands** 13 / 26 / 33% at low / medium / high | **all six scenarios within 2 points** (3 seeds) |
| Determinism with LL and failures | byte-identical |

## Calibrated crowd scenarios (`data/scenarios/*-crowd-*.yaml`, park mix, random timings, all failures)
| | Standby demand | LL demand | LL share | Standby wait | LL wait |
|---|---|---|---|---|---|
| Everest low / medium / high | 1,464 / 1,830 / 2,379 | 219 / 495 / 624 | 13 / 26 / 33% | 0.7 / 25.6 / 35.0 min | ~0.5 min |
| Rock 'n' Roller low / medium / high | 1,160 / 1,450 / 1,885 | 167 / 399 / 505 | 13 / 26 / 33% | 5.6 / 26.6 / 37.5 min | 5.5 / 13.7 / 13.9 min |

- The bands are a calibration of [assumed] LL demand, as the spec allows. Below the merge cap the share
  is demand over throughput, so the model's real job is the stress behavior: phase changes, recovery
  after downtime, and the cost to standby.
- **Rock 'n' Roller's LL wait (~14 min)** comes from the mandatory hallway, show and post-show line. That's
  a testable prediction: if real LL waits there are much shorter, either LL guests bypass part of the
  preshow path or the merge sits later than the hallway.

## Data gaps added
1. **Single-rider availability criteria** ([user]: not always open; unknown when). Scripted for now.
2. LL demand per crowd level and return-window shape (M7 replaces the curves with park attendance x passes).
3. Rock 'n' Roller LL wait observations, to check the merge position.
