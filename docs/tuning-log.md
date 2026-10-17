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
