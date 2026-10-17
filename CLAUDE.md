# ridesim — working agreements

Deterministic discrete-event theme park ride simulator in Go. Behavior is defined by `docs/spec.md`;
numbers come from `data/`. Read both before changing simulation logic.

## Stack
- Go 1.22+, module `github.com/jasrags/ridesim`. Standard library plus `gopkg.in/yaml.v3` only;
  any other dependency needs a written justification and Jason's approval.
- Layout follows spec §3: `cmd/ridesim`, `internal/{engine,config,ride,queue,guest,park,metrics}`.

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
- Values carry trailing comment tags: `[wiki] [rcdb] [fan] [map] [user] [assumed]`.
- Never change a sourced value to make a test pass. `[assumed]` values may be tuned, and every
  change goes in `docs/tuning-log.md` with before, after, and reason.
- Explain any validation miss before tuning anything.
- Any tool that rewrites YAML must preserve comments (yaml.v3 `Node` API). None exists yet; data edits
  are made by hand so the git diff stays reviewable.
- All files are `schema_version: "1.0"`; the loader rejects anything else.
- `config_invariants` strings are documentation; the real checks live in `internal/config/invariants*.go`
  with severity `error` (blocks loading) or `warn` (reported, loading continues).

## Workflow
- Tests first for validation targets and invariants (each error-level check has a broken fixture
  under `internal/config/testdata/`).
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
