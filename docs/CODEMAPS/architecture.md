<!-- Generated: 2026-10-19 | Files scanned: 150 (119 .go, 30 .yaml, 1 .html) | Token estimate: ~750 -->
# Architecture

Single Go module `github.com/jasrags/ridesim`: a deterministic discrete-event theme park ride
simulator. One binary (`cmd/ridesim`), no services, no database. Behavior spec: `docs/spec.md`.

## Layers (imports point down only)
```
cmd/ridesim  (CLI: validate run report study serve challenge osm-import)
   |-- web ------> live ------> scenario ----> park ----> ride/{coaster,continuous,elevator,bays}
   |-- study ----> scenario, metrics             |              |
   |-- challenge -> scenario, metrics, score     |              |
   |-- osm (offline import, no sim deps)         |              v
   |-- metrics --> park (types only)             |          ride/lines -> queue ; ride/seats
   v                                             v              v
 config (YAML schema, loader, invariants)      guest (parties, arrivals)
 engine (clock, (time,seq) scheduler, RNG streams, event log)  <- everything
```

## Data flow (one run)
```
data/*.yaml --config.LoadDir--> Dataset --scenario.Load--> Runner (Coaster|Continuous|Elevator|Bays|ParkRun)
Runner.Run() --> engine.Sim events --> engine.Log (append-only JSONL)
engine.Log --metrics.Compute / ComputePark--> report text   (metrics read the log only)
```

## Live play (serve)
```
browser <-SSE snapshots-- web.Server --mutex--> live.Session --RunUntil per tick--> engine.Sim
browser --POST lever/speed/jump--> ops applied between ticks, recorded as timed park ops
GET /api/export --> park scenario YAML that reruns byte-identically
```

## Determinism rules (CLAUDE.md)
- Time = `engine.Time`/`Duration` int64 ms; no wall clock, timers, goroutines in sim logic.
- RNG: one seed split into named streams; `Sim.Scope(src)` gives each park node its own streams.
- Scheduler order (time, seq); never iterate maps into output.
- Goroutines only outside sim logic: `study` replications (ordered by seed), `web` ticker.

## Ride archetypes (spec §6-8)
| archetype | package | rides |
|---|---|---|
| multi_train_coaster | ride/coaster | Everest, Rock 'n' Roller, Slinky, Rise, Runaway Railway |
| continuous_load | ride/continuous | Haunted Mansion, Smugglers Run, Toy Story Mania |
| elevator_drop | ride/elevator | Tower of Terror |
| simulator_bays | ride/bays | Star Tours |
| ride_simple / meet | park (activity.go) | remaining park nodes |
