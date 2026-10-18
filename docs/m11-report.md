# M11 results: Rise of the Resistance (2026-10-18)

Scope (Jason): give Rise of the Resistance a full model, the third headliner. Rise is a chain of group stages in front of a trackless
ride, so M11 adds **further batch stages** after a ride's preshow and runs the vehicles on the coaster model (Jason's choices:
stage chain + coaster model, ~1,540/hr counted capacity, failures fitted to ~11% downtime).

## The guest path (sourced)
| Stage | Sourced | Model |
|---|---|---|
| Briefing room | 2 mirrored rooms, "just over a minute" [fan disneygeek] | preshow: 2 rooms x 40, 75 s |
| Intersystem Transport Ship | standing simulator, 30–50 guests, "just over 2.5 minutes" [fan]; 3 rooms on a turntable [fan] | stage `transport_ship`: 3 rooms x 40, 150 s |
| Hangar walk | 50 stormtroopers [fan] | not modeled (adds time only) |
| Holding cell | groups of at most 16 [wiki], about a minute [fan] | stage `holding_cell`: 3 cells x 16, 60 s |
| Fleet Transports | 8 seats in 2 rows of 4 [wiki]; a cell's group boards two vehicles [wiki][fan]; 38 cars, 36 in the show space [fan] | pairs as 16-seat trains: fleet 19, max 18, 12 in service |
| Ride | ~5 min on the vehicles [fan]; 18 show areas, escape pod drop [wiki][fan] | 15 scene blocks + escape pod + pre-unload = 300 s |

Group sizes beyond the sourced 16, the cell count, the turnovers and the scene split are `[assumed]`. Every stage in front can carry
more than the ride: briefing 2,743, ship 2,400 and cells 2,304 per hour.

## What was built
- **Further stages (`stages:` in a coaster ride file).**
  - **Order:** each stage is a batch stage like the preshow (rooms, group size, show and turnover times, a holding area behind
    it), run in guest order after the preshow.
  - **Holding areas:** each stage releases into its own holding area (its `load_area_capacity`); the next stage fills from it in
    release order. The last stage releases into the load line.
  - **Back-pressure:** a full holding area blocks the rooms behind it, all the way back to the outdoor queue.
  - **Logging:** later stages log under their own id. The preshow stats in reports still cover only the preshow.
  - **Validation:** stages need the preshow before them and a unique id, only the preshow can be the Lightning Lane merge, and each
    stage must carry the listed capacity. Every error has a broken fixture.
- **Rise ride file** `data/rides/wdw-dhs-rise-of-the-resistance.yaml`: the station loads one pair every 26.4 + 8 + 3 = 37.4 s
  `[assumed]`, which is 1,540/hr.
- **Downtime fitted to the data:** long stops every 3.33 h (3.6 a day [fan BlogMickey]) lasting 19 ± 8 min, plus the usual brief
  stops. That is ~11.5% of a 12-hour day, against ~11% measured [fan TouringPlans].

## Checks
- **Capacity:** fixed times give 37.4 s and 1,540/hr. Every stage runs, and holding-cell shows match dispatches (one cell group
  fills one pair).
- **Back-pressure:** a 10-minute transport ship caps the ride at the ship's 686/hr, and the briefing rooms stall behind it.
- **Downtime:** over 20 random days Rise is stopped 9–14% of the time.
- **Full guest flow** (park mix, Lightning Lane, failures):
  - same seed, same log;
  - every guest who joined boarded, reneged or is still in the chain;
  - Little's Law holds to 0.9%.
- **Existing rides unchanged:** the synthetic park, Rock 'n' Roller, Haunted Mansion, Everest and DHS day logs were byte-identical
  before Rise's park node switched.

## Hollywood Studios (medium, 10 seeds, failures on)
In the park Rise carries ~1,220/hr, against 1,483 as a simple ride. Three things cost capacity:
- ~11% breakdowns;
- cells that close partly full because parties are never split;
- dispatch variance.

At appeal 1.09 it posted 52.7 / 68.4 min. Re-fit to 1.02 (approved):

| Posted avg / busiest hour (min) | Queue-Times 2026 | simple ride | full model, 1.02 |
|---|---|---|---|
| Rise of the Resistance | 47 / 65 | 45.8 / 61.0 | 47.1 / 61.7 |
| Slinky Dog Dash | 63 / 83 | 59.9 / 81.5 | 61.8 / 82.5 |
| Tower of Terror | 30 / 53 | 31.3 / 49.0 | 32.3 / 50.6 |
| Rock 'n' Roller | 45 / 63 | 43.8 / 57.8 | 44.6 / 59.0 |

- **Turn-aways:** about 3,400 guests a day turn away at Rise's entrance (3,240 as a simple ride).
- **Spillover:** the other rides rose 0.2–1.8 min, because guests who can't ride Rise during breakdowns go elsewhere. Tower of
  Terror now sits 2.3 min over target; it is noted here, not tuned.

## Code review
A Go review of the stage chain found nothing critical or high. Re-entrancy between stages is guarded, no stall path was found,
Lightning Lane and waiting counts stay correct, and the logs stay deterministic. Fixed:
- a holding area smaller than the next stage's group is now a validation error, since that stage's rooms could never fill;
- a backwards validation message.

Noted, not changed:
- **Drain order:** a stage with several rooms waiting to release drains them in room order, not in the order they finished, so
  group order through the chain is approximate. Changing it touches the shared preshow and would change Haunted Mansion's logs.
- **Stats:** blocked time is reported for the preshow only, not per later stage.
- **Quiet times:** at low crowds a later stage whose fill timer has run out shows the next group at once, so a cell can run nearly
  empty.

## Open
- **Not modeled:**
  - Rise down at park opening on ~25% of days [fan]: needs per-day failure draws.
  - The 3-room turntable: rooms are modeled as independent, not rotating together.
  - The hangar walk time.
  - The single rider line (not permanent, per BlogMickey Oct 2024).
- **Observations to collect `[user]`:**
  - group size in the briefing room and ship;
  - number of holding cells;
  - time between vehicle pairs;
  - scene timings;
  - where Lightning Lane merges.
