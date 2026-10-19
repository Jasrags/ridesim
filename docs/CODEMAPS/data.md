<!-- Generated: 2026-10-19 | Files scanned: 30 .yaml + engine/log.go | Token estimate: ~700 -->
# Data (no database: YAML inputs, JSONL event log out)

All files `schema_version: "1.0"`; strict decode; values carry provenance tags
`[wiki] [rcdb] [fan] [map] [osm] [user] [assumed]`. Files are read-only on disk.

## data/rides (10)  common keys: ride, failures, queues, [preshow], [stages], validation_targets, config_invariants
  coaster:    trains blocks clearance station [barn] [layout guest_path exit_path]
  continuous: conveyor route vehicles load_zone unload_zone loading_shared
  elevator:   vehicles sides cycle        bays: bays cycle
wdw-ak-expedition-everest, wdw-mk-haunted-mansion, wdw-dhs-{rock-n-roller-coaster, slinky-dog-dash,
rise-of-the-resistance, runaway-railway, tower-of-terror, smugglers-run, star-tours, toy-story-mania}

## data/parks (2)   top keys: park, nodes, edges, congestion, guests, decision, lightning_lane,
                     calibration_targets
park-synthetic.yaml
wdw-dhs.yaml (595 lines): node kinds ride 8, ride_simple 3, show 7, meet 3, dining 18, shop 5,
                          junction 140, entrance, exit, elsewhere. Geometry from OSM (ODbL).

## data/scenarios (16)  key: scenario{id, ride_id|park_id, seed, duration_h, warmup_min, crowd_level, attendance,
                         trains, fixed_times, failures, dispatch_rule, posted_wait, arrivals, ops, weather, guests,
                         lightning_lane, single_rider_closed, walking_mode, no_lightning_lane, accessibility_holds}
ride days: everest-*, rnrc-*, hm-baseline-day ; park days: park-{low,medium,high}-day, dhs-{low,medium,high}-day

## data/scoring (1)  key: scoring{id, experience{ride, show, meet}, wait_comfort_min, wait_penalty_per_min, balk_penalty,
  renege_penalty, walk_penalty_per_min, jam_penalty_per_min, ll_lapse_penalty}  (all [assumed], M15)
## data/challenges (3)  key: challenge{id, title, description, scenario, scoring, daily_seed, levers[], goals[{metric, node?,
  at_least | at_most}]}  metrics: score, experiences_per_guest, balks, reneges, ll_lapsed, ll_redeemed, posted_avg_min, posted_peak_min

## data/studies (3)  key: study{base, seeds, report_node, variants[{id, note, set | ride_set | scale}]}
everest-operations, hm-what-ifs, rnrc-guest-flow

## osm/wdw-dhs (outside data/, loader skips)
extract-2026-10-18.json (raw Overpass), import.yaml (mapping), query.overpassql

## Event log (engine.Event, one JSON object per line, field order fixed)
t, type, src, train, block, guest, n, d, info
types: run_start queue_join board train_enter hold_start hold_end hold_yield load_done dispatch unload
       ride_stop ride_resume cascade barn_swap train_to_barn train_from_barn ops_rejected balk renege
       single_rider posted_wait run_end (+ park-level types in internal/park)

## Docs
docs/spec.md (behavior), docs/tuning-log.md (every [assumed] change), docs/m1..m14-report.md
