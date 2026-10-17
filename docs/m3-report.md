# M3 results (2026-10-17)

Scope: Rock 'n' Roller (launch block, mid-course brake, preshow batch stage, single rider, parties,
abandonment). Everest picks up the same guest flow.

## Results against the targets
| Target | Result |
|---|---|
| Rock 'n' Roller 1,800/hr, 48 s, 4 trains (fixed times, saturated, single guests) | 48.0 s, 1,800/hr (after the pre-approved load_s 40 -> 37; as authored 51.0 s / 1,694/hr, matching hand math) |
| Four trains sufficient (lap 82 + 36 + 48 = 166 s < 4 x 48 s) | load station never idle |
| Preshow (130 per 180 s = 2,600/hr) is not the bottleneck | holds once load_area_capacity >= 72; at 48 the station starved (~1,492/hr) |
| Back-pressure from a slow station | load_s +20 s: release blocked > 1.5x longer, outdoor line > 2x longer, pace follows the station (68 s) |
| Parties never split; empty seats from odd parties; single riders fill them | park mix: 2.5 empty seats per dispatch, ~1,610/hr; single riders recover ~1% |
| Abandonment | balks and reneges logged; Little's Law holds with reneges as departures (0.4% over a drained surge) |
| Determinism | same seed, byte-identical log with the full guest flow and failures |

## Scenario days (seed 1, park party mix, 5% single riders, patience from the park file)
| Scenario | Riders/hr | Availability | Standby mean / p95 | Empty seats per dispatch | Cascades | Balks / reneges |
|---|---|---|---|---|---|---|
| rnrc_baseline_day | 1,425 | 96.6% | 19.6 / 28.3 min | 3.48 | 0 | 1,715 / 0 |
| everest_baseline_day | 1,828 | 98.2% | 17.3 / 23.0 min | 3.59 | 2 (yeti) | 1,483 / 77 |
| everest_ops_demo | 1,567 | 86.3% | 22.2 / 31.9 min | 3.06 | 26 (all yeti, during 4 trains) | 3,376 / 864 |

- Everest's day now averages 17.3 min standby, against the Queue-Times target of ~19 min. Balking caps
  the line, and the arrival steps are still [assumed].
- In the ops demo all cascade stops happen at YETI_BRAKE while 4 trains run. That matches the [fan] note
  in the Everest file ("rare holds at the yeti brake, except with fewer than 5 trains").
- Rock 'n' Roller's day loses 14.5% of seats to the party mix and partial shows (88% mean fill) when
  demand dips. Posted waits are within ~2 min of actual on both baseline days.

## Decisions made in M3
- Rock 'n' Roller max_in_service 5 -> 4 [fan] (web summary pasted by Jason). The spec's "five give
  slack" target is dropped.
- Seats are rows (`trains.seats_per_row`, [wiki] 2). Parties take whole rows. Gaps go to a head-of-line
  solo party or single riders; single riders also take empty rows after load_done.
- **Combined post-show line** [user]: after Rock 'n' Roller's preshow, parties and single riders share one
  line in release order; the grouper pulls the earliest single rider into gaps.
- `preshow.load_area_capacity` [assumed]: 130 (one show's release). 48 starved the station.
- The fill wait ends when nobody in line can fill a free seat. Without this, every not-quite-full train
  waited the full max_wait, which cost ~12% with the party mix.
- **Cascade scope `course_brakes`** (default, both rides): the brake feeding the station is the normal
  platform queue. Everest's authored 5 s works under it (~0.56 cascades/hr); the M2 30 s calibration was
  reverted. Rock 'n' Roller's mid-course brake never holds, so it sees ~0 cascades.
- Guest parameters come from scenario `guests` sections, which default to the park file
  (party sizes, patience). Single-rider share defaults to 0.05 [assumed].

## Bugs found and fixed
1. Preshow re-entrancy: Release -> board -> Drain re-entered Drain and emptied rooms repeatedly (561 shows
   instead of 11 in the regression test).
2. Arrival curve rates were parties/hr; every file and flag says guests/hr. Hidden until party sizes
   exceeded 1.
3. Riders-by-lane counted only guests who joined in the window. It now counts boardings in the window.

## Flags and open data
1. Rock 'n' Roller's preshow length (150 s), turnover (30 s) and load area (130) are [assumed]. Whether the
   observed 130 guests is the hallway or the room is still open (spec §13 gap #3).
2. With 4 trains, Rock 'n' Roller trains wait ~26 s every lap at PRE_UNLOAD_HOLD, which is station-bound.
   A cascade can therefore only come from the mid-course brake.
3. The single-rider share (5%) is [assumed]; observed shares would sharpen the empty-seat numbers.
4. The pasted summary calls the launch both LIM and LSM; the file keeps [wiki] LSM. Launch physics does
   not affect timing here.
