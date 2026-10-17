# Tuning log

Every change to an `[assumed]` value in `data/` is recorded here: file, field, before, after, reason, date.
Sourced values (`[wiki] [rcdb] [fan] [map] [user]`) are never tuned.

| Date | File | Field | Before | After | Reason |
|---|---|---|---|---|---|
| 2026-10-17 | wdw-ak-expedition-everest.yaml | blocks.LIFT.transit_s.mean | 45 | 40 | LIFT + PEAK_BRAKE + switch realign was 59 s > 54 s target; now 54 s. Approved by Jason (docs/m1-report.md). |
| 2026-10-17 | wdw-ak-expedition-everest.yaml | blocks.SHADOW_BRAKE.transit_s.mean | 12 | 20 | Absorbs time removed from the mountain-exit region; REVERSE + SHADOW region 40 s. |
| 2026-10-17 | wdw-ak-expedition-everest.yaml | blocks.MOUNTAIN_EXIT.transit_s.mean | 45 | 32 | MOUNTAIN_EXIT + HELIX region was 69 s (+2 s release = 71 s headway, 1,724/hr); now 52 s. |
| 2026-10-17 | wdw-ak-expedition-everest.yaml | blocks.HELIX_TO_BRAKES.transit_s.mean | 24 | 20 | Same region as MOUNTAIN_EXIT. |
| 2026-10-17 | wdw-ak-expedition-everest.yaml | blocks.YETI_BRAKE.transit_s.mean | 4 | 6 | Keeps the block sum at ride_duration_s (170 s). |
| 2026-10-17 | wdw-ak-expedition-everest.yaml | blocks.PRE_UNLOAD_HOLD.transit_s.mean | 14 | 26 | Keeps the block sum at 170 s; outside any multi-block clearance region. |
| 2026-10-17 | wdw-ak-expedition-everest.yaml | station.load_s.mean | 44 | 43 | Station cycle 43 + 8 + 3 = 54 s, matching the [fan] 54 s target interval. |

Result (fixed times, 5 trains, saturated): 71.0 s / 1,724 per hr -> 54.0 s / 2,264 per hr. Stochastic times: ~2,074 per hr
(listed capacity 2,050). Sensitivity after the retune: LIFT, load_s, and MOUNTAIN_EXIT + HELIX are each within 2 s of being the
bottleneck; see docs/m1-report.md.
| 2026-10-17 | wdw-ak-expedition-everest.yaml | failures.cascade_stop.blocked_tolerance_s | 5 | 30 | Pre-approved calibration (M2). At 5 s cascades fire ~65/hr and availability is 16%; below ~30 s stops feed on themselves. With scope brakes_only, 30 s gives ~0.45 cascades/hr and ~2,012/hr (80 h x 3 seeds). Jason chose 30 s brakes-only. |

M2 note: the listed 2,050/hr is not reachable by cascade tuning. Failures off gives ~2,067/hr with random timings, and brief_stop +
long_downtime ([assumed]) cost 2.7% (-> ~2,011/hr). Accepted as a 1.9% miss against a Wikipedia design figure (decision 2026-10-17).
| 2026-10-17 | wdw-dhs-rock-n-roller-coaster.yaml | station.load_s.mean | 40 | 37 | Pre-approved (M3). Sim reproduced the hand math as authored: 40 + 8 + 3 = 51 s, 1,694/hr (4 trains, fixed times). Now 48 s, 1,800/hr = listed theoretical. |
| 2026-10-17 | wdw-dhs-rock-n-roller-coaster.yaml | preshow.load_area_capacity | 48 | 130 | New [assumed] field. At 48 the 180 s show cycle starved the station (preshow became the bottleneck, ~1,492/hr), which spec §11 rules out; >= 72 lets the station set the pace. Jason chose one show's release (130). |
