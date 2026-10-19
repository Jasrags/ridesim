# ridesim

A deterministic discrete-event simulator for theme park rides and whole park days, written in Go.

It models how real attractions move guests: coaster blocks and station dispatch, omnimover chains,
drop-tower cabs, simulator bays, preshows, standby / single-rider / Lightning Lane lines, breakdowns,
and guests choosing what to ride next as they walk the park. The same seed always produces a
byte-identical event log, and every metric is computed from that log.

Modeled today: Disney's Hollywood Studios (every headliner has a full ride model), Expedition Everest,
Haunted Mansion, and a synthetic test park.

## Run it

Requires Go 1.22 or newer. The only dependency is `gopkg.in/yaml.v3`.

```sh
go test ./...
go run ./cmd/ridesim validate                       # load every data file and check its invariants
go run ./cmd/ridesim run -scenario data/scenarios/dhs-medium-day.yaml -log out/dhs.jsonl
go run ./cmd/ridesim report -log out/dhs.jsonl      # recompute metrics from a saved log
go run ./cmd/ridesim serve -scenario data/scenarios/dhs-medium-day.yaml   # live control room at http://127.0.0.1:8080/
go run ./cmd/ridesim study -file data/studies/everest-operations.yaml     # A/B study on paired seeds
go run ./cmd/ridesim serve -challenge data/challenges/dhs-tame-slinky.yaml # play a challenge (score and goals)
go run ./cmd/ridesim challenge -file data/challenges/dhs-tame-slinky.yaml -session played.yaml -name you  # verify onto the leaderboard
```

## Layout

- `cmd/ridesim` — the CLI (`validate`, `run`, `report`, `study`, `serve`, `challenge`, `osm-import`)
- `internal/` — the engine, config loader, ride archetypes, queues, guests, park model, metrics, live
  session and web server; see [`docs/CODEMAPS/architecture.md`](docs/CODEMAPS/architecture.md)
- `data/` — rides, parks, scenarios, studies, the score weights and challenges as YAML
- `docs/spec.md` — the behavior spec; `docs/m*-report.md` — validation results per milestone;
  `docs/tuning-log.md` — every change to an assumed value

## Where the numbers come from

Every value in `data/` carries a source tag: `[wiki]`, `[rcdb]`, `[fan]` (fan sites and forums),
`[map]` (the park guide map), `[osm]` (OpenStreetMap), `[user]` (direct observation) or `[assumed]`.
Only `[assumed]` values are tuned, and each change is logged with its reason.

## License

The code is MIT licensed; see [LICENSE](LICENSE). Park geometry in `data/parks/wdw-dhs.yaml` and the
extract in `osm/` are © OpenStreetMap contributors under the
[ODbL 1.0](https://www.openstreetmap.org/copyright).

This is an unofficial fan project. It is not affiliated with or endorsed by The Walt Disney Company;
park and attraction names are used only to identify what is being modeled.
