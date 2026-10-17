# M2 results (2026-10-17)

Scope: failures and cascade stops, barn swaps, dispatch rule variants, posted-wait algorithms, plus a
minimal versioned scenario file (pulled forward from M6 to hold scripted inputs).

## What was built
- **Stop model** (decision: run to next brake). Stop causes nest. Station work, brake transits and
  release delays pause and resume with their remaining time. Trains on lift and course blocks run on to the
  next brake or station. Causes: `brief_stop` (Poisson), `long_downtime` (exponential MTBF, normal MTTR),
  `cascade_stop`, `weather_hold` (scripted), `barn_swap`.
- **cascade_stop**. Only `next_occupied` holds count, and only at brakes (`failures.cascade_stop.scope:
  brakes_only`, new validated field). A hold still running after `blocked_tolerance_s` stops the ride for
  `operator_reset_s`. Designed waits never count.
- **Barn swaps** (scripted `add_train` / `remove_train`). Each swap stops the ride for `swap_time_s`. Fleet,
  `max_in_service` and storage bays are checked by the scenario validator and again at runtime. The
  shadow-room hold switches off below 5 trains.
- **Dispatch rules**: `hold_for_full_or_max_wait`, `dispatch_when_ready`, `hold_for_full` (scenario override).
- **Posted wait**: `queue_len_over_throughput` (15 min observed throughput, theoretical fallback),
  `last_n_observed`, `padded`. Logged every 300 s. Metrics compare what each guest saw at join with their
  actual wait.
- **Scenario files** (`data/scenarios/*.yaml`, schema 1.0): ride, seed, duration, warmup, arrival steps,
  failure kinds, overrides, ops events, weather windows. `ridesim run -scenario <file>`.
- **Metrics**: downtime per cause and as a union, availability, lost riders, cascades by block, swaps,
  empty seats per dispatch, stacking time at brakes, posted-vs-true error and bias.

## Calibration (5 trains, random timings, 80 h x 3 seeds, demand 2,300/hr)
| Setting | Riders/hr | Availability | Cascades/hr |
|---|---|---|---|
| No failures | 2,067 | 100% | 0 |
| Brief + long downtime only | 2,011 | 97.3% | 0 |
| Cascade, brakes + stations, 5 s (as authored) | 369 | 16% | 65 |
| Cascade, brakes + stations, 30 s | 1,909 | 92% | 4.4 |
| **Cascade, brakes only, 30 s (chosen)** | **2,012** | **97.3%** | **0.45** |

- Below about 30 s the ride collapses: each reset leaves trains stacked behind one another, which sets
  off the next cascade. This sharp threshold is a model property worth remembering when M8 exposes
  tolerance as a game lever.
- **Listed 2,050/hr is a 1.9% miss, accepted** (decision 2026-10-17). It can't be reached by tuning cascades,
  because brief stops and long downtime ([assumed]) already cost 2.7%.
- Station-time randomness (`load_s`/`unload_s` sd 8 s, [assumed]) is the biggest single lever. It costs 9%
  (2,264 fixed -> 2,067 random).

## Scenario results
- **everest_baseline_day** (seed 1): 2,030 riders/hr, 98.8% availability, 2 cascade stops (both
  PRE_UNLOAD_HOLD), standby mean 29.4 min / p95 39.7, posted error 1.7 min. Little's Law within 0.9% over
  the day.
  - The standby average is above the Queue-Times target (~19 min). That's driven by the [assumed] arrival
    steps; calibrate with the park model (M7).
- **everest_ops_demo** (seed 2): 2 swaps, 4 trains from 03:00 to 05:00 with no shadow-room holds,
  a 30 min weather hold. 88.9% availability, 1,777 riders/hr, posted error 9.3 min. Posted wait lags
  sharply around the closure, which is the herding effect M7 needs.

## Tests added
Pause exactness, run-to-brake timing, nested causes, no boarding while stopped, failure rates vs
Poisson bounds, cascade trigger and reset, cascade scope table, barn add/remove and limits, the three
dispatch rules, the posted-wait algorithms and refresh, a hand-built metrics log (overlapping causes,
window clipping), scenario schema checks with broken fixtures, weather cost, ops demo, random-failure
derate vs hand math (97.3%), determinism with failures on, calibration regression.

## Flags
1. Rock 'n' Roller still has `blocked_tolerance_s: 8` and no scope. It defaults to brakes only now and
   needs its own calibration in M3.
2. With `hold_for_full`, a party that never fits would stall the train. That isn't possible with M1's
   single-guest parties; M3 parties need single-rider fill or a cap.
3. A barn swap stops the whole ride for the swap time. This is my modeling assumption, not data.
