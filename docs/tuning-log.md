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
| 2026-10-17 | wdw-ak-expedition-everest.yaml | failures.cascade_stop.blocked_tolerance_s | 30 | 5 | Reverted to the authored value. M3 found that holds at the brake feeding the station are the normal platform queue (Rock 'n' Roller waits ~26 s there every lap). With scope course_brakes (Jason's decision) the authored 5 s gives ~0.56 cascades/hr and 96.7% availability on Everest (park party mix, 60 h x 3 seeds). |
| 2026-10-17 | both coasters | failures.cascade_stop.scope | brakes_only / (none) | course_brakes | [user] decision, not a tuned value; recorded here because it replaced the tolerance calibration above. |
| 2026-10-17 | scenarios/*-crowd-{low,medium,high}.yaml | lightning_lane arrivals (guests/hr) | (new) | Everest 219 / 495 / 624; Rock 'n' Roller 167 / 399 / 505 | Pre-approved M4 calibration: LL demand per crowd level so the realized LL share lands in the [fan] 13 / 26 / 33% bands (park mix, random timings, all failures, 3 seeds x 10 h). Standby demand is 80 / 100 / 130% of capacity ([assumed]). |
| 2026-10-17 | park-synthetic.yaml | nodes.elsewhere (visit_min 45/20, appeal 0.60, thrill 0.40) + edge hub_elsewhere (800 x 80 ft) | (new) | as listed | New [assumed] node (M7, Jason's decision): the rest of the park, so attendance stays park-wide while the modeled rides hold ~25% of guests. Approved diff. |
| 2026-10-17 | park-synthetic.yaml | boat_ride / flat_ride queue_capacity | (new) | 800 / 300 | New [assumed] field: physical queue before spillback onto hub_boat / hub_flat. Approved diff. |
| 2026-10-17 | park-synthetic.yaml | decision.meal_window_bonus | (new) | 0.8 | New [assumed] field: dining utility boost inside a meal window until the party has eaten. Approved diff. |
| 2026-10-17 | park-synthetic.yaml | lightning_lane (purchase 0.35, advance 3, window 60 min, grace 15 min, inventory 0.30, elsewhere 0.70) | (new) | as listed | New [assumed] section for park-run pass issuance; advance booking then rebook-after-use is [user]. Approved diff. |
| 2026-10-17 | park-synthetic.yaml | nodes.elsewhere.visit_min | mean 45, sd 20 | mean 30, sd 15 | M7 calibration (approved). Long elsewhere visits made 26% of LL bookings lapse (now ~17-20%) and kept guests off the modeled rides. |
| 2026-10-17 | park-synthetic.yaml | lightning_lane.elsewhere_share | 0.70 | 0.20 | M7 calibration (approved). Realized LL share was 8-12% at medium; too few bookings landed on the modeled rides. |
| 2026-10-17 | park-synthetic.yaml | lightning_lane.inventory_share | 0.30 | 0.40 | M7 calibration (approved). Lets high crowds reach the 33% band. |
| 2026-10-17 | park-synthetic.yaml | lightning_lane.purchase_share | 0.35 | 0.30 | M7 calibration (approved). Brings low crowds near the 13% band. |

M7 calibration result (10 seeds per crowd level, failures on, congestion walking; long downtime closes the ride and is skipped in posted
stats with 15 min recovery, Jason's decision): medium posted avg / peak hour Everest 21.5 / 36.4 min (target 19 / 35), HM 28.8 / 50.7
(target 26 / 42). Realized LL share Everest / RnR / HM: low 12.5 / 16.7 / 14.4%, medium 22.9 / 29.4 / 26.0%, high 30.1 / 33.9 / 36.8%
(bands 13 / 26 / 33). HM's peak hour runs high on posted-wait spikes after chain stops and accessibility holds (docs/m7-report.md).
