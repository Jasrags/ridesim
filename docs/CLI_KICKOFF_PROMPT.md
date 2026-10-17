We're building a theme park ride simulator in Go. I'm Jason, a senior engineering manager who has built Go CLIs and event-driven systems for years, so skip the basics and show your reasoning on design choices.

Everything you need is in this directory:
- docs/spec.md: the spec (v0.2). Read all of it first.
- data/rides/*.yaml: three ride configs (Expedition Everest, Rock 'n' Roller Coaster, Haunted Mansion at Walt Disney World). These are the source of truth for numbers.
- data/parks/park-synthetic.yaml: a made-up park that composes those rides, for later milestones.

## Step 0: read, then stop
Read the spec and every data file. Do not write code yet. Then give me:
1. A plan for M0 and M1 in 15 lines or fewer.
2. Your blocking questions. Ask them with the question tool (clickable options) rather than free text, one decision per question.
Wait for my answers before building.

## How we work
- Go 1.22 or newer, standard library plus gopkg.in/yaml.v3 only unless you justify another dependency. Module path github.com/jasrags/ridesim.
- Create a CLAUDE.md in the repo root that captures these working agreements, so future sessions pick them up.
- Determinism is a hard requirement. Simulated time is int64 milliseconds. Use one seeded RNG split into named streams (arrivals, failures, guest choice, station times). No wall clock and no map-iteration order in simulation logic. A test must show two runs with the same seed produce byte-identical event logs.
- The YAML files carry provenance as trailing comment tags: [wiki], [rcdb], [fan], [map], [user], [assumed]. Never change a sourced value to make a test pass. [assumed] values may be tuned, but log every change in docs/tuning-log.md with the before and after and the reason. If any tool rewrites a YAML file it must preserve comments (use the yaml.v3 Node API).
- The config_invariants strings in the YAML files are documentation. Implement them as Go checks with a severity: error blocks loading, warn prints and continues. The Haunted Mansion check that vehicles.count x spacing_s is within 15% of the sum of route est_s currently fails by about 22%; that is a known data question, so it must be a warning.
- Tests first for the validation targets. Keep `go test ./...` green before moving to the next step. Commit in small steps.
- Anything that is a real design choice (not a detail) goes to me through the question tool.

## M0: loader and validator
- Typed config structs for both archetypes (multi_train_coaster, continuous_load) and the park file. Dispatch on ride.archetype and schema_version.
- The three ride files currently have schema versions 0.5, 0.5, and 0.4, and the park file has none. Propose normalizing everything to one version (I suggest 1.0), update the data files, and make the loader reject unknown versions.
- `ridesim validate` loads every file under data/, resolves park ride_id references, runs all invariants, and prints a clear pass/warn/fail report. Include the data_conflicts entries in the report.
- Tests for each invariant, including a deliberately broken fixture for each error-level check.

## M1: engine core and Everest
- Engine: clock, scheduler with a (time, seq) tiebreak, named RNG streams, append-only event log that can be replayed.
- Multi-train coaster model driven by data/rides/wdw-ak-expedition-everest.yaml: blocks with brake kinds, clearance rules, pivot switches with realign time, station cycle, dispatch rule, and the shadow-room hold_until_clear behavior. Fixed arrival curve and a standby queue only. Leave failures, barn swaps, preshow, Lightning Lane, and park-level crowds for later milestones, but keep the Ride interface from the spec so they plug in.
- A metrics report from the event log: realized dispatch interval, throughput per hour, wait mean/p50/p95, brake hold time per brake block.
- Acceptance, with failures off and fixed times: five trains give a dispatch interval of about 54 s and about 2,267 riders/hr (3600/54 x 34); four trains show the gap where the fifth would be; same-seed logs are byte-identical; average queue length matches arrival rate x average wait (Little's Law) at steady state. Explain any miss before tuning anything.

## Things to flag to me as you go
- Any place a data file contradicts the spec or itself.
- The Rock 'n' Roller duration conflict (82 s vs 70 s) and the Haunted Mansion spacing mismatch (3.6 s assumed vs about 3.0 s implied by the route).
- Which [assumed] values most influenced the results.

## Out of scope for now
Front ends, the park-level crowd model, Lightning Lane merge phases, the continuous_load model, and real-park data beyond these three rides. Keep the interfaces ready for them but don't build them.

When M0 and M1 pass, summarize results against the targets, list the assumptions that mattered most, and stop for my review.
