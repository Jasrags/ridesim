# Theme Park Ride Simulator — Spec v0.2

Supersedes v0.1. This version folds in everything decided while building the first three ride configs and the synthetic park.
Data files live in `data/rides/` and `data/parks/`. They are the source of truth for numbers; this document is the source of truth for behavior.

## 1. Goals
- Deterministic, headless discrete-event simulation of theme park ride operations, queues, and crowds.
- Model rides deeply (block systems, brakes, dispatch, preshow stages, merge ratios), then compose them into a park.
- Two front ends on the same engine, later: a live management game and an analytical what-if tool. The engine must not depend on either.
- Engine language: Go (stdlib plus a YAML library). Front-end platform is still undecided.

## 2. Non-goals (v1)
- Visual realism, 3D, ride physics beyond what drives timing.
- Full park economics (food, retail, ticketing revenue).
- Multiplayer.

## 3. Architecture
```
cmd/ridesim/          CLI: validate, run, report
internal/engine/      clock (int64 ms), scheduler, named RNG streams, event log
internal/config/      YAML types, versioned loader, invariants (error/warn)
internal/ride/        Ride interface; coaster/ and continuous/ implementations
internal/queue/       standby, single rider, preshow batch stage, skip-line merge, posted wait
internal/guest/       arrivals, parties, patience, utility choice
internal/park/        node/edge graph, walking (fixed_delay | congestion), show and dining nodes
internal/metrics/     collectors derived from the event log
data/rides/           one YAML per ride, filename = ride id with dots replaced by dashes
data/parks/           park layouts that reference rides by ride_id
docs/                 this spec and the kickoff prompt
```
Principles:
- Determinism: same scenario + seed = byte-identical event log. No wall clock, no map-iteration order in sim logic.
- Everything observable is an event; metrics are derived from the event log.
- Ride models implement one interface so archetypes plug in:
  ```go
  type Ride interface {
      ID() string
      Arrive(p *Party, lane Lane)      // Lane: Standby, SingleRider, LightningLane
      PostedWait() time.Duration       // what guests see, not the true wait
      Status() Status                  // open, down, weather_hold, ...
  }
  ```
- Archetype is chosen by `ride.archetype` in the YAML: `multi_train_coaster` and `continuous_load` exist today.

## 4. Engine core
- `Clock`: simulated time as int64 milliseconds (recommended; confirm early).
- `Scheduler`: priority queue of `(time, seq, event)`; `seq` breaks ties deterministically.
- `RNG`: one seeded source split into named streams (arrivals, failures, guest choice, station times) so changing one stream does not shift the others.
- `EventLog`: append-only, serializable, replayable.

## 5. Config conventions
### 5.1 Ride identity
- `ride.id` is `<resort>.<park>.<ride>` (for example `wdw.mk.haunted_mansion`). `ride.family` groups versions of the same ride across parks; `ride.other_versions` is informational.
- Each park's version is its own file with its own id because layouts and capacities differ. The park file references rides by `ride_id`, never by filename.

### 5.2 Provenance tags
Values carry a trailing YAML comment tag. Tags are comments, not parsed fields:
`[wiki]` Wikipedia/infobox, `[rcdb]` RCDB, `[fan]` forums and aggregators, `[map]` Park Lore ride maps and official park guide maps (names, lands, what exists), `[osm]` OpenStreetMap geometry (positions, path lengths; ODbL, © OpenStreetMap contributors), `[user]` observed or confirmed by Jason, `[assumed]` our placeholder, tune later.
- Never change a sourced value to make a test pass. `[assumed]` values may be tuned; record each change in `docs/tuning-log.md`.
- If tooling rewrites a YAML file it must preserve comments (use the yaml.v3 Node API).
- `ride.data_conflicts` records disagreeing sources, the values, and which one was chosen.

### 5.3 Schema versions (as authored)
| File | schema_version | Notes |
|---|---|---|
| wdw-ak-expedition-everest.yaml | 0.5 | brake blocks, barn with 5 bays |
| wdw-dhs-rock-n-roller-coaster.yaml | 0.5 | launch block, preshow, brakes, barn with unknown bays |
| wdw-mk-haunted-mansion.yaml | 0.4 | continuous_load, route, split load/unload zones |
| park-synthetic.yaml | none | park config v0.1 |

Version history: 0.2 preshow, launch block, data_conflicts, invariants; 0.3 continuous_load archetype; 0.4 route, load_zone/unload_zone split, closed loop; 0.5 `brake` block kind (replaces `show_hold` and `brake_hold`). Early task: normalize all files to one version (proposed 1.0) and make the loader reject unknown versions.

### 5.4 Invariants
The `config_invariants` strings in each file document checks the loader implements in Go (they are not evaluated from the strings). Each check has a severity:
- **error** blocks loading: block times sum to ride duration (+/- 2 s), `count(blocks) >= max_in_service + 1`, every `next` and clearance reference exists, every brake block has its fields, preshow throughput >= listed capacity, `default_in_service <= max_in_service <= fleet_size`.
- **warn**: Haunted Mansion's `vehicles.count * spacing_s ~= sum(route.est_s)` currently misses by about 22% (576 s vs 473 s). That is a known data question, not a bug; keep it a warning until spacing is observed.

## 6. Archetype: multi-train coaster
### Track
- Ordered blocks with kinds: `station`, `lift`, `launch`, `course`, `brake`. Each block holds at most one train; a train advances only when the next block is clear.
- `brake` blocks: `hold_capable`, `stop_policy: stop_if_next_occupied` (otherwise the train passes at transit speed), `release_delay_s` after the next block clears. `transit_s` on a brake is its nominal pass-through time when not held.
- `contains` lists track switches inside a block (Everest has two pivot switches); `switch_realign_s` is the time for a switch to return before a train may be cleared.
- `clearance` rules: a train may leave block X only when the listed blocks are empty.
- `hold_until_clear` (Everest shadow room): hold a train until the station train advances; ignored when fewer than 5 trains run.

### Stations and dispatch
- Cycle = unload + load + restraint check + dispatch clearance. Parameters per file: staff, check time, target and minimum dispatch interval, dispatch rule (`hold_for_full_or_max_wait`).
- Separate load and unload areas are modeled as separate station blocks.

### Fleet and barn
- `fleet_size`, `max_in_service`, `default_in_service`. Trains not in service sit in the `barn` and can be swapped in (`swap_time_s`). `storage_bays: null` means unknown; assume enough.

### Failures
- `cascade_stop`: a train blocked longer than `blocked_tolerance_s` (measured at brakes and stations) halts the ride; `auto_resume: false` means an operator reset (`operator_reset_s`) is required. This is Everest's main real-world downtime driver with 5 trains.
- `brief_stop`, `long_downtime` (MTBF/MTTR), scripted `weather_hold`.

## 7. Archetype: continuous load (omnimover)
- No trains, blocks, or dispatch interval. A chain of `vehicles.count` vehicles moves at `conveyor.speed_fps`; capacity comes from vehicle spacing x occupancy.
- `route`: ordered scenes with map-scaled timings (load, scenes, unload, return run). Timings are `[assumed]`; the scene order is `[map]`.
- Separate `load_zone` and `unload_zone`. `loading_shared.accessibility_hold` freezes the whole chain (carpet and buggies) at a given rate and duration.
- `party_vehicle_policy: contiguous_vehicles`: parties larger than a vehicle occupy consecutive vehicles, never interleaved with other parties.
- A stop halts everything at once; there is no cascade stacking. The queue absorbs it.

- Chains (M12): `conveyor.chains` identical independent loops (Smugglers Run's turntables), staggered, each with its own load point;
  `side_down` freezes one. `party_vehicle_policy: shared`: parties fill `party_seats` front seats, single riders the rest, then parties,
  with grouper callouts from up to `callout_parties` back.

### 7a. Archetype: elevator drop (M10, Tower of Terror)
- Mirrored `sides`; each has `show_shafts`, `drop_shafts` and `unload_doors`, and `vehicles.per_side` self-propelled cabs that stay on their side.
- A cab's lap (`cycle`): unload door -> show shaft (lift one floor to load) -> load -> show scenes -> drop shaft -> unload door. It holds
  each place until the next one is free; waiters are served first come, first served. Validation keeps cabs per side below the number of
  places, so the cycle cannot gridlock.
- Seating follows the coaster's row rule (`seats_per_row`); the load line, Lightning Lane merge and preshow are the shared queue model.
- `ride_duration_s` = `show_s + drop_s + to_unload_s` (dispatch to the unload door). The capacity check (warn) compares sides x seats x 3600
  over the slowest per-side resource with `listed_capacity_per_hr`.
- A ride stop freezes every cab with its remaining time. `side_down` freezes one side only (logged as `side_down` / `side_up`, not
  `ride_stop`), so capacity halves and the ride stays open.

## 8. Queue model
- Further batch stages (M11): a coaster ride file may list `stages:` after its `preshow`, in guest order. Each is a batch stage with
  the preshow's fields plus an `id`; each releases into a holding area of its `load_area_capacity`, which the next stage fills from
  in release order, and the last releases into the load line. Back-pressure runs back through the chain. Only the preshow can be the
  Lightning Lane merge point.
Flow: arrival -> balk check -> standby / single rider / Lightning Lane -> preshow (if any) -> load -> ride.
- **Standby**: FIFO of parties with size, patience, join time. Finite physical capacity; overflow spills onto the adjoining walkway in the park model.
- **Balking and reneging**: balk on arrival if posted wait > tolerance; renege mid-line if wait exceeds tolerance (lazy deletion).
- **Party policy**: `never` split across trains/batches (decided). Leftover seats go to single riders or stay empty. Omnimover parties use contiguous vehicles.
- **Single rider**: own FIFO; fills empty seats at load. On Rock 'n' Roller, single riders also pass through the preshow hallway and show (confirmed by observation).
- **Preshow batch stage** (Rock 'n' Roller, Haunted Mansion stretching room): rooms with state FILLING -> SHOWING -> RELEASING -> EMPTY. Starts when full or after `max_fill_wait_s`; parties are never split. RELEASING blocks while the load area is full, which is how a slow station lengthens the standby line. Rock 'n' Roller: 1 room, 130 guests per show `[user]`. Haunted Mansion: 2 rooms x 60 guests `[assumed]`.
- **Skip-the-line merge**: Lightning Lane (LL) merges with standby at the merge point (Rock 'n' Roller: preshow hallway; Everest: station queue entrance; Haunted Mansion: preshow entrance). Phases chosen by LL queue length with hysteresis: normal (<60 guests) 2:1, elevated (<200) 4:1, recovery (>=200) 10:1, in LL:standby guests `[fan, assumed]`. If the LL queue is empty, pull from standby. The unit (guests vs parties) is configurable.
- **Realized LL share of capacity is an output**, expected about 13% (low crowds), 26% (medium), 33% (high) `[fan]`. If the sim misses that band, tune LL demand or phase thresholds.
- **Posted wait**: separate from true wait; default `queue_len_over_throughput`; alternatives `last_n_observed` and `padded`. Log both.

## 9. Park-level crowd model
Config: `data/parks/park-synthetic.yaml` (a made-up composite park holding the three real ride models plus filler nodes, a theater, and dining). Real parks come later.
- **Graph**: nodes (rides, shows, dining, plaza, entrance, exit) and edges with length and width. Rides plug in by `ride_id`; `ride_simple` nodes are capacity/duration filler.
- **Guests**: individual party agents (decided). Arrivals follow a time-of-day curve with a rope-drop spike scaled by a crowd-level multiplier; party size distribution; patience, thrill tolerance, stay length, fatigue, LL passes.
- **Choice**: softmax over top-k utility = appeal + thrill match - wait penalty (uses the posted wait, refreshed every 300 s so guests herd on stale numbers) - walk penalty - repeat penalty + LL return-window bonus; meal windows boost dining.
- **Walking**: `congestion` mode is the default for the park file (decided); `fixed_delay` must stay available through the same `TravelTime` interface. Link time is BPR style: `t = t0 * (1 + alpha * (n / C)^beta)`, `C = length x width x comfortable_density`; jam density is a hard cap. Ride queue overflow consumes walkway width (spillback).
- **Behaviors to reproduce**: wait spikes when a show lets out near a ride (theater exit link into Haunted Mansion), breakdown ripple effects, LL return clustering.
- **Calibration targets** `[fan]` (Queue-Times averages, not ground truth): Everest standby about 19 min average / 35 peak; Haunted Mansion about 26 / 42. Calibrate rides with `fixed_delay` first, then enable congestion and retune.

## 10. Metrics
- Throughput: theoretical vs actual riders per hour; realized dispatch interval.
- Wait: mean, p50, p95; posted vs true error.
- Station/chain utilization, empty seats per dispatch/vehicle, brake hold time per brake block, stacking time.
- Preshow: batch fill, partial-batch rate, time blocked in RELEASING.
- Merge: realized LL share, phase occupancy, LL and standby waits.
- Abandonment: balks and reneges. Downtime: count, duration, lost throughput.
- Park: edge congestion, in-park population, time per activity.

## 11. Validation targets
Hand math the sim must reproduce with no failures and fixed times, before any tuning:
- **Everest** (5 trains): dispatch about 54 s -> theoretical 3600/54 x 34 = about 2,267 riders/hr; listed capacity 2,050 (about 90%) should emerge once real-world losses are on. Expect brake holds at the shadow room, pre-unload hold, and peak under a load delay, and rare holds at the yeti brake. Four trains should leave a gap where the fifth would be.
- **Rock 'n' Roller** (4 trains default): implied 48 s interval -> 3600/48 x 24 = 1,800/hr. Cycle about 182 s (82 s ride + about 100 s station) against 4 x 48 = 192 s, so four trains are marginally sufficient; five give slack. Preshow throughput (1 room x 130 per 180 s = about 2,600/hr) must not be the bottleneck.
- **Haunted Mansion**: 3600 / spacing_s x occupancy ~= 2,400/hr at spacing 3.6 s and 2.4 riders per buggy; hard ceiling 3600 / spacing x 3 seats. Accessibility holds must measurably reduce throughput.
- **Little's Law**: average queue length = arrival rate x average wait at steady state.
- **Determinism**: same seed twice gives byte-identical event logs.
- **Crossover**: more trains raise throughput until the station is the bottleneck, then brake holds and stacking rise.
- **Merge**: realized LL share lands in the 13/26/33% bands at low/medium/high crowds.

## 12. Milestones
- **M0**: repo scaffold, config types, versioned loader, invariants with severities, `ridesim validate` over every data file; normalize schema versions.
- **M1**: engine core + Everest coaster model, fixed arrival curve, standby only, metrics, CLI report; passes the Everest hand-math, determinism, and Little's Law tests.
- **M2**: failures and cascade stops, barn swaps, dispatch rule variants, posted-wait algorithms.
- **M3**: Rock 'n' Roller: launch block, mid-course brake, preshow batch stage, single rider, parties, abandonment.
- **M4**: Lightning Lane merge phases and realized-share output.
- **M5**: continuous_load archetype (Haunted Mansion): spacing, occupancy, accessibility holds, contiguous parties.
- **M6**: scenario files and A/B comparison output (analysis tool backbone).
- **M7**: park-level crowd model on the synthetic park: graph, agents, congestion, spillback, show-exit spikes.
- **M8**: first front end (platform decision), live controls for the game.

## 13. Data gaps and known inconsistencies
Values to collect on future park visits, roughly in order of impact:
1. Haunted Mansion: real buggy spacing and average riders per buggy; stretching room size and number of rooms.
2. Everest: where trains actually pause, and per-section times (lift, reverse, drop, helix).
3. Rock 'n' Roller: preshow show length, hallway refill time, storage bay count and swap time; whether the observed 130 is the hallway or the room.
4. Merge ratios: LL guests per standby group at each ride's merge point.
5. Per-scene timings on Haunted Mansion (the route timings are map-scaled guesses, about +/-30%).

Known conflicts recorded in the data files: Rock 'n' Roller duration (82 s Wikipedia vs 70 s RCDB, chose 82); Haunted Mansion capacity (2,400 chosen; sources run 2,400 to over 3,000), duration, track length (960 vs 1,400 ft, unused), and seats per vehicle; Haunted Mansion spacing 3.6 s (capacity-derived) vs about 3.0 s (route-derived).

## 14. Open questions
- Time representation: int64 ms recommended, not yet confirmed.
- Scenario/ops-lever file format (YAML is implied by the ride configs).
- Normalize schema versions to 1.0 now, or after M1.
- Front-end platform (terminal UI vs web).
- How closely to model real safety interlock rules vs the simplified block-clearance model used here.
