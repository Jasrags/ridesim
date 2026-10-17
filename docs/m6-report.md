# M6 results (2026-10-17)

Scope: scenario files (built in M2-M5) and A/B comparison output: the backbone of the analysis tool.

## What was built
- **Study files** (`data/studies/*.yaml`, schema 1.0). A study names a base scenario and variants. Each
  variant can use:
  - `set`: deep-merged into the scenario (`trains: 4`, `single_rider_closed: [...]`)
  - `ride_set`: deep-merged into the ride file in memory, so data is never rewritten
    (`vehicles: { spacing_s: 3.0 }`)
  - `scale`: multiplies arrival rates (`lightning_lane: 1.3`)

  `validate` resolves every variant and runs every data check on it, so a bad override fails up front.
- **Paired replications.** By default each variant runs on seeds 1-20, the same seeds as the base. The
  named RNG streams make these common random numbers, so a variant that changes one lever sees the same
  arrivals, guests and failures.
- **Statistics.** Each metric gets a mean with a 95% CI, plus a paired difference against the base
  (95% t interval, `*` when it excludes zero). Metrics: riders/hr, availability, standby wait mean and
  p95, LL share, LL wait, balks, reneges, empty seats, posted-wait error, max queue.
- **Output**: a text table, plus JSON with every replication for the future what-if front end.
  `ridesim study -file data/studies/<x>.yaml [-seeds N] [-json out.json]`.
- **Speed**: replications run in parallel (one single-threaded sim per goroutine; results ordered by seed,
  and tested to be independent of parallelism). 60 Everest days take ~0.6 s.

## Example study results (20 paired seeds)
**everest_operations** (medium crowd)
| Variant | Riders/hr | Standby wait | Balks |
|---|---|---|---|
| base | 1,834 +/- 24 | 26.6 min | 4,093 |
| four_trains | **-205 +/- 13 (-11.2%)** | +5.7 min (+21%) | +2,188 (+54%) |
| faster_load (load_s 43 -> 40) | +41 +/- 14 (+2.2%) | -0.5 min | -313 |

**hm_what_ifs** (baseline day). These are the open [assumed] values.
| Variant | Riders/hr | Availability | Standby wait | Posted error |
|---|---|---|---|---|
| base | 1,694 | 82.4% | 12.0 min | 2.5 min |
| no_accessibility_holds | +51 (+3.0%) | +15.1 pts | **-7.7 min (-64%)** | -1.5 min |
| half_accessibility_holds | +44 (+2.6%) | +7.0 pts | -5.1 min (-42%) | -0.9 min |
| route_spacing_3s | +48 (+2.8%) | +0.0 (identical stop draws) | -7.0 min (-58%) | -0.8 min |

The baseline day is demand-limited, so these levers barely move throughput but move waits a lot. Both
unknowns matter about equally for waits; either one is worth observing.

**rnrc_guest_flow** (medium crowd)
| Variant | Standby wait | LL share | Balks |
|---|---|---|---|
| base | 27.9 min | 26.8% | 3,430 |
| ll_plus_30pct | +4.0 min (+14%) | +8.1 pts | +1,225 (+36%) |
| single_rider_closed | +0.3 min (+1%) | +0.2 pts | +86 |

Closing Rock 'n' Roller's single-rider line costs little (-0.6% riders) at a 5% single-rider share.

## Notes
- Paired differences are much tighter than the base CI itself: Everest riders/hr base +/-24 vs differences
  +/-13. That is the payoff of common random numbers.
- Studies change values only in memory. Adopting a what-if into data is still a tuning-log decision.
