# ridesim — working agreements

Deterministic discrete-event theme park ride simulator in Go. Behavior is defined by `docs/spec.md`;
numbers come from `data/`. Read both before changing simulation logic.

## Stack
- Go 1.22+, module `github.com/jasrags/ridesim`. Standard library plus `gopkg.in/yaml.v3` only;
  any other dependency needs a written justification and Jason's approval.
- Layout: `cmd/ridesim`, `internal/{engine,config,ride,queue,guest,park,metrics}` (spec §3), plus
  `internal/{scenario,study,live,web,osm,score,challenge}` and ride archetypes under `internal/ride/`. Map:
  `docs/CODEMAPS/architecture.md`.

## Determinism (hard requirement)
- Simulated time is `engine.Time` / `engine.Duration`: int64 milliseconds. Never use `time.Now`,
  `time.Since`, timers, or goroutines in simulation logic.
- All randomness comes from one seeded root split into named streams (`arrivals`, `failures`,
  `guest_choice`, `station_times`, ...). Never use the global `math/rand` functions.
- Never let map iteration order reach simulation logic or output; iterate sorted keys or slices.
- Scheduler order is `(time, seq)`; `seq` is assigned at schedule time.
- Every observable change is an event in the append-only log; metrics are derived from the log only.
- A test must keep showing that two runs with the same seed produce byte-identical event logs.

## Data files and provenance
- Values carry trailing comment tags: `[wiki] [rcdb] [fan] [map] [osm] [user] [assumed]`.
- Never change a sourced value to make a test pass. `[assumed]` values may be tuned, and every
  change goes in `docs/tuning-log.md` with before, after, and reason.
- Explain any validation miss before tuning anything.
- Any tool that rewrites YAML must preserve comments (yaml.v3 `Node` API). None exists yet; data edits
  are made by hand so the git diff stays reviewable.
- All files are `schema_version: "1.0"`; the loader rejects anything else.
- `config_invariants` strings are documentation; the real checks live in `internal/config/invariants*.go`
  with severity `error` (blocks loading) or `warn` (reported, loading continues).

## Workflow
- Tests first for validation targets and invariants (each error-level check has a test that patches a
  valid fixture from `internal/config/testdata/` to break it).
- Keep `go test ./...` green before moving to the next step. Small commits, conventional messages
  (`feat:`, `fix:`, `test:`, `docs:`, `chore:`, `refactor:`).
- Real design choices go to Jason as a multiple-choice question; details are decided and noted.
- Flag any place a data file contradicts the spec or itself.

## Decisions log (M0/M1 kickoff, 2026-10-17)
- Clearance rules are applied literally. If targets miss, report the cause and propose tuning; do not tune
  without sign-off.
- `station.target_dispatch_interval_s` is a validation target, not a pacing input. Only
  `min_dispatch_interval_s`, the dispatch rule, and clearance gate dispatch.
- Schema 1.0: each clearance rule names its source block (`from:`) and extra conditions are typed
  (`conditions: [...]`).
- Everest `hold_until_clear`: a train held in the shadow room is released once an UNLOAD->LOAD advance
  happens after it arrived, or immediately if UNLOAD was empty when it arrived. Applies only with >= 5 trains.
- Station block handoff (UNLOAD -> LOAD) takes 0 s of move time (no data); revisit if data appears.
- `hold_until_clear` also yields when another held train needs that block empty (avoids the 5-train circular
  wait the [fan] rules create; the real ride runs 5 trains). Yields are logged as `hold_yield`.
- The Everest [assumed] retune (approved 2026-10-17) is in the YAML and `docs/tuning-log.md`; the pre-retune
  values live in test code (`preTuneValues`) to keep the literal-clearance bottleneck under test.
- Data files are read-only on disk; unlock, edit, relock (`chmod u+w` / `chmod a-w`) only with approval.

## Decisions log (M2 kickoff, 2026-10-17)
- cascade_stop counts unplanned holds only: `next_occupied` at **course brakes**, i.e. every brake except
  the one feeding the station, where trains normally queue for the platform (`scope: course_brakes`, the
  default; changed in M3). Designed waits never trigger it. Everest keeps its authored 5 s (the M2 30 s
  calibration was reverted); the 2,050/hr listed capacity is an accepted ~1.9% miss, see docs/m2-report.md.
- Ride stop = run to next brake: trains on lift/course/launch finish to the next brake or station and stop
  there; station work, brake transits and release delays pause and resume with their remaining time.
- Barn swaps are scripted ops events. A swap stops the ride for swap_time_s (cause barn_swap).
- Scripted inputs (arrivals, ops events, weather windows, failure kinds, overrides) live in a versioned
  scenario YAML under `data/scenarios/`, validated like every other data file. M6 extends it.

## Commands
- `go test ./...`
- `go run ./cmd/ridesim validate [-data data]`
- `go run ./cmd/ridesim run -ride wdw.ak.expedition_everest -trains 5 -seed 1 -fixed-times -hours 4 -log out/events.jsonl`
- `go run ./cmd/ridesim report -log out/events.jsonl`
- `go run ./cmd/ridesim run -scenario data/scenarios/everest-baseline-day.yaml`
- `go run ./cmd/ridesim study -file data/studies/everest-operations.yaml -json out/study.json`
- `go run ./cmd/ridesim run -scenario data/scenarios/park-medium-day.yaml -log out/park.jsonl` (park day, M7)
- `go run ./cmd/ridesim serve -scenario data/scenarios/park-medium-day.yaml` (web control room, M8; http://127.0.0.1:8080/)
- `go run ./cmd/ridesim run -scenario data/scenarios/dhs-medium-day.yaml` (Hollywood Studios day, M9; serve works the same)
- `go run ./cmd/ridesim serve -challenge data/challenges/dhs-tame-slinky.yaml` (challenge play, M15; daily ones take -date)
- `go run ./cmd/ridesim challenge -file data/challenges/dhs-tame-slinky.yaml [-session played.yaml -name NAME]` (baseline, or verify onto out/leaderboard.json)
- `go run ./cmd/ridesim osm-import -config osm/wdw-dhs/import.yaml -out out/dhs-draft.yaml -svg out/dhs-draft.svg` (draft park from OSM, M9)

## Decisions log (M3 kickoff, 2026-10-17)
- Rock 'n' Roller max_in_service 5 -> 4 [fan] (web summary: a 5th train stacks). The spec's "five give
  slack" target is dropped; check that 4 trains suffice instead.
- Rock 'n' Roller load_s 40 -> 37 [assumed] is pre-approved once the sim reproduces the 51 s / ~1,694/hr
  hand math (log it in tuning-log.md).
- Seats are rows (`trains.seats_per_row`): a party takes whole rows; leftover seats go to a solo party at
  the head of the standby line (no skipping) or to single riders, who may also take empty rows after load_done.
- Preshow back-pressure uses `preshow.load_area_capacity` [assumed]: 130 on Rock 'n' Roller (one show's
  release; 48 starved the station). The post-preshow line is one combined line [user]: release order is kept,
  single riders at the front board in order, and the grouper pulls the earliest single rider to fill gaps.
- Rock 'n' Roller preshow [user]: the 130-guest batch gathers in a hallway (`preshow.staging: hallway`) while
  the previous group watches the ~2-3 min show (150 s), then walks in when the room frees.
- Guest parameters (party sizes, single-rider share, patience) are scenario fields that default to the
  park file's guests section. Ad hoc runs without them keep single guests and no abandonment.

## Decisions log (M4 kickoff, 2026-10-17)
- Lightning Lane demand is a scenario arrival curve (guests/hr, [assumed]); smooth Poisson for now, return-window
  clustering arrives with the park model (M7). LL guests never balk or renege.
- The merge is a credit counter in the configured unit with phase hysteresis; credit accrues only while LL guests
  wait. Merge points: Rock 'n' Roller preshow hallway, Everest the load line (station queue entrance).
- The standby posted wait is standby-side guests / standby riders per second (15 min window).
- The LL share bands (13/26/33%) are calibrated by LL demand per crowd level (pre-approved; tuning log).
- Single-rider lines are not always open ([user]; criteria unknown): scenarios script closure windows.

## Decisions log (M6, 2026-10-17)
- A/B studies live in `data/studies/*.yaml`: a base scenario plus variants with `set` (scenario deep-merge),
  `ride_set` (ride-file deep-merge, in memory only) and `scale` (arrival multipliers). Every variant is resolved
  and checked like a data file during `validate`.
- Default 20 paired seeds per variant (common random numbers via named streams); report paired differences
  with 95% t intervals. Output: text table plus JSON (`ridesim study -file ... -json out.json`).
- Replications run in parallel goroutines, one single-threaded sim each; results are ordered by seed, so
  output never depends on scheduling (tested). Simulation logic itself stays goroutine-free.

## Decisions log (M7 kickoff, 2026-10-17)
- The synthetic park's modeled attractions hold only ~25% of a medium-day crowd at realistic waits. An
  `elsewhere` park node stands for the rest of the park (unlimited capacity, duration distribution, appeal);
  attendance stays park-wide and guests come and go from it.
- Lightning Lane in park runs ([user]): purchasing parties book up to `advance_bookings` passes before the day
  (random booking order, earliest window with inventory at or after their arrival), then book one more each time
  a pass is used or a window lapses. Hourly inventory is a share of ride capacity. The "2 hours after park open"
  rebooking rule is not modeled. Park runs replace the scripted LL curves; ride-only scenarios keep them.
- Calibration toward the Queue-Times targets may tune park-file [assumed] values only (utility weights,
  temperature, elsewhere, LL inventory, congestion), logged in tuning-log.md and reported before applying.
  Ride files are not touched.
- One simulation hosts many rides: `Sim.Scope(src)` stamps events with `src` (the park node id) and gives each
  scope its own RNG streams (`src/name`), so a ride's randomness does not depend on its neighbours.
- Long downtime closes a ride (M7, Jason's decision): the board shows it closed so guests do not choose it, guests who
  arrive anyway turn away (balk, info closed), and posted-wait stats skip the closure plus 15 min after reopening, as
  Queue-Times does. Brief stops are unchanged.
- M7 calibration targets the medium crowd (Jason's decision); the approved changes are in tuning-log.md.
- Fatigue raises the walk penalty only; departure follows stay_hours (Jason's decision). Fatigue-driven departure
  is a later refinement. Results, gaps and the recommended startup-cascade follow-up are in docs/m7-report.md.
- Everest cascade storms after a stop (and the startup burst) stay as modeled (Jason, 2026-10-17): the [fan] source
  says operators reset cascades constantly. Revisit with [user] counts of stops per hour and reset time.

## Decisions log (M8 kickoff, 2026-10-17)
- Front end: both, web first (Jason). M8 builds a web control room (Go standard-library HTTP server, plain
  HTML/SVG/JS page) on a shared event-stream API; a terminal console on the same API comes later. Mockups:
  https://claude.ai/artifact/A5DmkWDkphfcvzCJ2y8YeB
- Live play (M8): `ridesim serve` owns one park sim in a server goroutine and advances it with RunUntil per tick;
  sim logic stays single-threaded. Snapshots stream over server-sent events; controls are POSTs applied between
  ticks at the current sim time and recorded as timed park ops.
- Park node coordinates are x/y fields in the park file (Jason). A played session exports as a park scenario with
  those ops and reruns byte-identically. Jumping back to time T reruns the day from open with the ops before T
  (later ops are discarded).
- Crowd scale thins a day drawn at 2x attendance: each party keeps a fixed uniform draw and arrives only while it is
  below scale / 2, so changing the scale never shifts other draws.

## Decisions log (M9 kickoff, 2026-10-18)
- M9 builds a real park, Disney's Hollywood Studios, before any game features (Jason). Sources: the 2026 guide map
  (names, lands, what exists today) and OpenStreetMap (real-scale positions and path lengths).
- New provenance tag `[osm]` for OpenStreetMap geometry. OSM data is ODbL: park files derived from it carry
  "© OpenStreetMap contributors", and the derived data stays ODbL if the repo is ever published.
- Rides go in as `ride_simple` with sourced capacity and duration first, so the whole park runs and calibrates; headliners
  then get full models one at a time (Slinky Dog Dash first, it fits multi_train_coaster). Rock 'n' Roller keeps its model.
- Nodes: every ride, show, character meet and dining location; shops grouped one node per land; `elsewhere` shrinks to
  resting and wandering.
- Import: a dated raw OSM extract is committed under `osm/<park>/` (outside `data/`, so the loader skips it), and a
  stdlib-only `ridesim osm-import` writes a draft park YAML from it plus a hand-edited mapping file. The draft is reviewed, then edited by hand; the tool never rewrites an existing file.
- Path graph: keep junctions where three or more paths meet plus attraction nodes; straight-through segments merge into one
  edge with the summed length.
- Widths: OSM has none, so widths default by path type `[assumed]`; known chokepoints get measured later `[user]`.
- Simple-ride capacity is the operational figure, ~90% of the fan-listed capacity (crooksinwdw OHRC; Tower of Terror uses
  the 1,680/hr forum calculation) (Jason). It serves standby and Lightning Lane together; appeal is then calibrated.
- Lightning Lane at Hollywood Studios has tiers (Jason): Multi Pass books 1 Tier 1 + 2 Tier 2 before the day, then rebooks
  any Multi Pass ride after each use or lapse; Rise of the Resistance is a separate Single Pass with its own purchase
  share. Nodes declare `lightning_lane: tier1 | tier2 | single_pass`; a park with no tiers keeps the M7 model.
- Simple rides get a Lightning Lane line: each vehicle boards LL guests first, up to 2/3 of its seats (the spec's normal
  2:1 merge ratio), so standby always moves.
- Attendance (Jason): medium 28,300 a day (TEA 2024 average, derived), low 18,500 and high 38,000 [assumed]. Calibrate the
  medium day against Queue-Times 2026 averages.
- Character meets are `meet` nodes (one party at a time, meet_s per party); continuous films run as simple rides.
- Walkway gridlock fix (Jason): `congestion.min_capacity_length_ft` (100 ft at DHS) so short links carry crowds like a longer
  walkway, and `decision.reroute_s` (300 s at DHS) so routes follow current walking times. Both default off, so the synthetic
  park is unchanged. Simple rides and meets log their posted waits at each board refresh.
- DHS calibration (approved): patience 75 +- 30, wait penalty 0.012, ride and meet appeals fitted to Queue-Times 2026 (appeal
  may exceed 1.0). Results and gaps in docs/m9-report.md; changes in docs/tuning-log.md.
- Slinky Dog Dash full model (Jason, 2026-10-18): `data/rides/wdw-dhs-slinky-dog-dash.yaml`, 4 trains x 18 seats, no barn, no single
  rider line. Station times [assumed] for 50 s / 1,296/hr (90% of 1,440 listed). In the park it carries ~1,031/hr (odd parties leave
  seats empty, dispatch variance), so its appeal was re-fit 1.25 -> 1.15 (approved). Results in docs/m9-report.md.

## Decisions log (M10, Tower of Terror, 2026-10-18)
- New archetype `elevator_drop` (Jason; spec §7a, `internal/ride/elevator`): sides of show shafts, a drop shaft and unload doors, with
  cabs that hold each place until the next is free. Reuses the shared lines, the coaster seating rule and the stop model; adds
  `side_down` (one side out, capacity halves). `side_down` is not enabled in the DHS day scenarios: its rate is [assumed] and the
  halving is an unverified lead.
- Tower of Terror: 2 sides x (2 show shafts, 1 drop shaft, 2 doors), 4 cabs of 21 per side [wiki][fan]; cycle times [assumed], fitted
  to the forum's 90 s per side (1,680/hr) with the drop shaft as the bottleneck. Library preshow [user]: two rooms off the lobby that open
  into the boiler room, sized like Haunted Mansion's (60 a show, [assumed]).
- Seats are modeled one by one (`seats_per_row: 1`): parties share the 7-seat benches [assumed].

## Decisions log (M11, Rise of the Resistance, 2026-10-18)
- Rise is a stage chain plus the coaster model (Jason): briefing room = preshow; transport ship and holding cells = further batch
  `stages` (spec §8); the two vehicles boarding from one 16-guest cell run as one 16-seat train through scene blocks. Stages log under
  their own id; report preshow stats stay first-stage only, so existing logs are unchanged.
- Capacity built to TouringPlans' counted ~1,540/hr (Jason): one pair every 37.4 s. Failures fitted to ~11% downtime and ~3.6 stops a
  day (Jason): long_downtime mtbf 3.33 h, mttr 19 +- 8 min. Rise appeal re-fit 1.09 -> 1.02 (approved). Results in docs/m11-report.md.
- Runaway Railway (Jason): 2,160/hr (90% of listed), two preshow rooms of 32, parties share the 4-seat rows; trains of 32 on scene
  blocks, no new code. Jason's seating observations (five boarding groups of 3/4/3/4/7, callouts) were about Tower of Terror, not
  Railway.
- Tower of Terror seating [user] (Jason): five boarding groups of 3/4/3/4/7; a party sits in one group if any fits, else it is split;
  spare seats go to the next party, single riders, then a party called forward from up to `callout_parties` (10, [assumed]) back.

## Decisions log (M12, Smugglers Run, 2026-10-18)
- continuous_load extended (Jason): `conveyor.chains` (independent loops, staggered), `party_vehicle_policy: shared` with
  `party_seats` and grouper `callout_parties`, `side_down` per chain, `stages`, and the single-rider preshow bypass. Haunted Mansion
  unchanged.
- Smugglers capacity emerges from the sourced 45 s rotation (Jason): ~1,800/hr with the park mix. Crew rule [user] (one observation):
  parties fill the four front seats, single riders the engineers. Appeal re-fit 0.90 -> 0.96 (approved). Results in docs/m12-report.md.

## Decisions log (M13, Star Tours, 2026-10-18)
- New archetype `simulator_bays` (Jason: each bay launches on its own schedule from boarding and loading times): bays cycle exit ->
  board -> check -> safety video + flight; one shared line; `side_down` = one bay. Seating rules moved to `internal/ride/seats`
  (rows, boarding groups + callouts), shared with elevator_drop.
- Star Tours built to 1,620/hr (90% rule = crooksinwdw operational): 6 bays x 40, 533 s cycle, only the flight sourced. No re-fit.

## Decisions log (M14, Toy Story Mania, 2026-10-18)
- Toy Story Mania on continuous_load chains (Jason): 3 tracks, an 8-seat tram every 40 s per track (2,160/hr, 90% rule), shared trams
  with callouts. The continuous capacity ceiling checks the build target when set. Every DHS ride except Alien Swirling Saucers now has
  a full model; whole-park results in docs/m14-report.md.
- Fixes pass (Jason, 2026-10-18): park runs map brief_stop -> chain_stop for continuous rides; new failure `opening_delay` (Rise: 25%
  of days, 45 +- 30 min) enabled in the DHS days, Rise long stops 19 -> 16 min and appeal 1.02 -> 1.06 (approved). Partial outages
  stay off until data; preshow drain order unchanged.

## Decisions log (M15 kickoff, guest score and challenges, 2026-10-19)
- The game roadmap starts (Jason): the guest score and challenges come first, on Hollywood Studios, with a local leaderboard
  where a submitted session is rerun to verify its score.
- The score rewards guest experience only (Jason): points per guest for rides, shows and meets completed, minus waits past a
  comfort threshold, balks and reneges, walking and jam time, and lapsed LL passes. It is computed from the event log only;
  every weight is [assumed], lives in a scoring data file and is logged in tuning-log.md.
- M15 adds two Lightning Lane levers (Jason), because DHS has almost nothing else to pull: `ll_inventory` (share of a ride's
  capacity sold per window, for windows not yet started) and `ll_merge` (LL guests per standby guest; replaces the authored
  merge phases or the simple-ride 2/3 cap). Both trade off inside the score; no invented costs.
- Finding (2026-10-19): the LL levers barely move the whole-day score (48.4 +- 0.1 per guest on the medium day); they shift who
  rides, not how many. M15 challenges therefore set goals on ride waits the levers can move, with the score as a floor
  (Jason). Capacity and crowd-steering levers come with the trade-off milestone. Balks and reneges score only at rides,
  shows and meets (dining and shops earn nothing and have no levers).

## Decisions log (M16 kickoff, Hollywood Studios realism, 2026-10-19)
- Before another park (Jason): fix turn-aways (guests walking to a ride only to balk, ~30,000 a day), research the missing DHS
  data (theater sizes, restaurant seats, meet times, LL purchase share, 2025 attendance), add the missing guide-map places, the
  Animation Courtyard rename and the Vacation Fun closure, and a full Alien Swirling Saucers model. [user] widths and ride
  timings wait for Jason's next visit.
