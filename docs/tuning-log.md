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
stats with 15 min recovery, Jason's decision; after the code-review stream fixes): medium posted avg / peak hour Everest 23.4 / 46.1 min
(21.2 / 35.6 without the seed-6 cascade storm; target 19 / 35), HM 29.2 / 56.0 (target 26 / 42). Realized LL share Everest / RnR / HM:
low 12.4 / 16.4 / 14.3%, medium 23.2 / 29.1 / 26.1%, high 30.1 / 33.7 / 37.0% (bands 13 / 26 / 33). HM's peak hour runs high on posted-wait spikes after chain stops and accessibility holds (docs/m7-report.md).
| 2026-10-17 | park-synthetic.yaml | nodes.*.x / y | (new) | mockup layout | New [assumed] map positions for the M8 front end (Jason's decision); synthetic park, no real map. Not simulation inputs. |
| 2026-10-18 | wdw-dhs.yaml | (new file) | — | — | Hollywood Studios park (M9, approved). Every [assumed] value there is a first placeholder; changes from here on are logged below. |
| 2026-10-18 | wdw-dhs.yaml | guests.patience_min | mean 40, sd 15 | mean 75, sd 30 | M9 calibration (approved). With 40 min most guests turn away from lines that average 63 min (Slinky, Queue-Times 2026). |
| 2026-10-18 | wdw-dhs.yaml | decision.utility.wait_penalty_per_min | 0.03 | 0.012 | M9 calibration (approved). At 0.03 a 60-min line loses to a snack, capping headliner waits near 25 min; 0.03 fit the synthetic park's 19-26 min targets. |
| 2026-10-18 | wdw-dhs.yaml | ride appeal: slinky_dog_dash, rock_n_roller_coaster, runaway_railway, rise_of_the_resistance, tower_of_terror, toy_story_mania, smugglers_run, alien_saucers, star_tours | 0.95, 0.90, 0.85, 0.95, 0.85, 0.80, 0.80, 0.60, 0.55 | 1.25, 1.31, 1.10, 1.09, 1.07, 1.07, 0.90, 0.49, 0.50 | M9 calibration (approved): six rounds of 5 seeds, each moving appeal by 0.5 x 0.012 x (target - sim avg posted min). Rock 'n' Roller's thrill 1.0 vs the guests' mean tolerance 0.55 costs ~0.27 utility, hence the highest appeal. |
| 2026-10-18 | wdw-dhs.yaml | meet appeal: meet_mickey_minnie, meet_edna | 0.65, 0.40 | 0.66, 0.49 | Same fit (Olaf unchanged at 0.50). |

M9 calibration result (10 seeds, medium, failures on, congestion walking with the 100 ft capacity floor and 5-min rerouting): posted
avg / busiest hour vs Queue-Times 2026 avg / avg daily max: Slinky 60.2 / 76.6 (63 / 83), Rise 45.1 / 60.3 (47 / 65), Rock 'n' Roller
43.6 / 58.5 (45 / 63), Runaway Railway 31.6 / 48.2 (34 / 45), Tower of Terror 28.2 / 43.5 (30 / 53), Toy Story Mania 27.8 / 43.2
(30 / 51), Smugglers Run 27.6 / 40.9 (29 / 47), Saucers 15.4 / 24.5 (16 / 30), Star Tours 6.5 / 16.4 (7 / 13), meets Mickey & Minnie
29.4, Olaf 15.1, Edna 13.8 avg (31, 15, 14). Vacation Fun is not fitted (Disney posts a fixed 10 min). Details in docs/m9-report.md.
| 2026-10-18 | wdw-dhs-slinky-dog-dash.yaml | (new file) | — | — | Full coaster model for Slinky Dog Dash (approved). Station times are [assumed], set for the 50 s / 1,296/hr target Jason chose (90% of 1,440 listed): load 39 + restraint 8 + clearance 3 = 50 s, unload 24 s. Fixed times hit 50 s / 1,296/hr; random times 51.6 s / 1,253/hr; 3 trains 65 s / 990/hr. |
| 2026-10-18 | wdw-dhs.yaml | nodes.slinky_dog_dash appeal | 1.25 | 1.15 | Re-fit after the switch to the full model (approved). In the park the coaster carries ~1,031/hr against 1,248/hr as a simple ride: 2-seat rows with no single rider line leave ~1.5 of 18 seats empty per train (odd parties), and random station times give 52.9 s between trains. At 1.25 Slinky posted 69.4 / 97.6 min (target 63 / 83). 1.08 gave 54.4 / 71.7. |

Slinky full-model result (10 seeds, medium, as above): Slinky 60.8 / 85.6 (63 / 83), turn-aways 4,569 a day (4,911 as a simple ride);
every other ride within 0.8 min of the M9 result (all slightly up), meets within 1.2 min (Olaf 15.1 -> 16.3 against 15).
| 2026-10-18 | wdw-dhs-tower-of-terror.yaml | (new file) | — | — | Full elevator_drop model for Tower of Terror (M10, approved archetype). Every cycle time is [assumed], fitted to the forum's 90 s per side (1,680/hr) with the drop shaft as the bottleneck; preshow rooms [user], group size 60 [assumed]. Fixed times 1,680/hr; random times with failures ~1,540/hr. No park re-fit: medium posted 31.3 / 49.0 min against 30 / 53 (was 28.8 / 44.0 as a simple ride), appeal unchanged at 1.07. |
| 2026-10-18 | wdw-dhs-rise-of-the-resistance.yaml | (new file) | — | — | Full Rise model (M11, approved design): briefing preshow + transport ship and holding cell stages + 16-seat vehicle pairs on scene blocks. Station times [assumed] for one pair every 37.4 s (~1,540/hr counted [fan], Jason). long_downtime fitted to ~11% downtime [fan TouringPlans] and ~3.6 stops a day [fan BlogMickey]: mtbf 3.33 h, mttr 19 +- 8 min. Fixed times 1,540/hr; stopped 9-14% over 20 days. |
| 2026-10-18 | wdw-dhs.yaml | nodes.rise_of_the_resistance appeal | 1.09 | 1.02 | Re-fit after the switch to the full model (approved). In the park Rise carries ~1,220/hr against 1,483 as a simple ride (breakdowns ~11%, partly filled cells, dispatch variance) and posted 52.7 / 68.4 min (target 47 / 65). 1.02 gives 47.1 / 61.7; 0.97 gave 43.5 / 54.8. |
| 2026-10-18 | wdw-dhs-runaway-railway.yaml | (new file) | — | — | Full Runaway Railway model (approved choices: 2,160/hr = 90% of listed, two preshow rooms of 32, parties share rows). Station times [assumed] for 53.3 s; preshow film 80 s + 15 s turnover [assumed] so two rooms carry 2,400/hr. No park re-fit: medium posted 34.4 / 51.5 min against 34 / 45, appeal unchanged at 1.10. |
| 2026-10-18 | wdw-dhs-tower-of-terror.yaml | vehicles.boarding_groups, cycle.callout_parties | seats one by one (seats_per_row 1) | [3, 4, 3, 4, 7], 10 | [user] seating observed by Jason (five boarding groups, parties kept together where a group fits, spare seats filled, callouts for a party size; cabs almost always full). Callout reach of 10 parties is [assumed]. Medium posted 32.0 / 49.7 -> 30.1 / 46.9 min (target 30 / 53); ~1,524 -> ~1,598 riders/hr. |
