<!-- Generated: 2026-10-19 | Files scanned: 119 .go | Token estimate: ~950 -->
# Backend (CLI, sim packages, HTTP API)

## CLI (cmd/ridesim, dispatch table in main.go)
validate   -> config.LoadDir + invariants            (validate.go)
run        -> scenario.Load(file) | ad hoc coaster -> Runner.Run -> metrics report, -log JSONL (run.go)
report     -> engine.ReadLog -> metrics.Compute / ComputePark                                  (run.go)
study      -> study.Run(base + variants, paired seeds) -> table + JSON                          (study.go)
serve      -> live.New(session) -> web.Server on 127.0.0.1:8080                                 (serve.go)
osm-import -> osm build -> draft park YAML + SVG (never overwrites)                             (osmimport.go)

## HTTP API (internal/web/server.go)
GET  /            -> embedded static/index.html
GET  /api/state   -> live.Snapshot JSON
GET  /api/events  -> SSE snapshot stream
POST /api/speed   -> set sim speed
POST /api/lever   -> Session.Apply(config.OpsEvent)
POST /api/jump    -> Session.JumpTo(t)  (rerun from open with earlier ops)
GET  /api/export  -> Session.Export -> park scenario YAML

## Packages (non-test lines)
engine (303)   sim.go: New(seed), Scope(src), At/After, RunUntil, RunBefore, Stream(name), Emit
               log.go: Event{T,Type,Src,Train,Block,Guest,N,D,Info}, Log.Write, ReadLog
config (3.5k)  types*.go schema; load.go LoadBytes/LoadFile/LoadDir (strict, schema 1.0);
               invariants*.go per archetype + park (error|warn); scenario.go; study.go (deep-merge variants)
scenario (718) Runner{Run, WarmupDuration}; Load(ds, file) picks CoasterRun|ContinuousRun|ElevatorRun|BaysRun|ParkRun
park (1.8k)    park.go New/Start/hostRide; agent.go party choice; activity.go simple rides, meets, shows;
               graph.go + walk.go walkways, congestion; ll.go Lightning Lane tiers; ops.go; view.go
ride (38)      interface Ride{ID, Arrive, PostedWait, Status}
ride/lines     shared front of house: standby / single rider / LL lines, merge, preshow, load line
ride/seats     rows rule + boarding groups with callouts
ride/coaster   blocks, brakes, clearance, station pipeline, barn, cascade, stops, posted wait
ride/continuous conveyor chains, shared vehicles, chain stops
ride/elevator  sides, show shafts, drop shaft, doors, side_down
ride/bays      per-bay exit -> board -> check -> flight cycle
queue          FIFO with lazy delete, preshow batch stage, LL merge credit
guest          Party, Poisson piecewise arrivals, party sizes, patience
metrics        metrics.go ride report; ops.go; park.go ComputePark (posted waits, closures, edges)
live           Session: Advance, Apply, JumpTo, Ops, Export, Snapshot; feed.go event ticker
study          Run -> replicate (parallel, ordered by seed) -> paired diffs, 95% t intervals
osm            extract.go Overpass JSON; graph.go junction merge; build.go; emit.go YAML/SVG
