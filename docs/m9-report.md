# M9 results (2026-10-18)

Scope (Jason): build a real park before game features. M9 models Disney's Hollywood Studios. The sources are the 2026 guide
map (names, lands, what exists today) and OpenStreetMap (real-scale positions and path lengths). Attraction numbers come from
a sourced research pass. The Hollywood Studios day runs in `ridesim run` and in the web control room.

## How to run
```
go run ./cmd/ridesim run -scenario data/scenarios/dhs-medium-day.yaml          # also dhs-low-day, dhs-high-day
go run ./cmd/ridesim serve -scenario data/scenarios/dhs-medium-day.yaml        # http://127.0.0.1:8080/
go run ./cmd/ridesim osm-import -config osm/wdw-dhs/import.yaml -out out/dhs-draft.yaml -svg out/dhs-draft.svg
```

## What was built
- **`ridesim osm-import` and `internal/osm`.** Stdlib only. It turns a saved Overpass extract plus a hand-edited mapping file
  (`osm/<park>/import.yaml`) into a draft park YAML and an SVG review map. It never overwrites an existing file. The steps:
  1. **Select guest walkways.** Backstage (`access=private`) paths and queue lines are dropped. Paths that lie entirely
     inside listed outlines (Fantasmic's seating aisles) are dropped too.
  2. **Join pieces.** Dead ends within `snap_ft` of another path are bridged; OSM paths often stop a few feet short. Pieces
     with no dead ends, such as plaza outlines, are joined to the nearest other piece within `snap_ft`. Hand-reviewed `links`
     cover real gaps in the mapping: Fantasmic's walkway is linked to Sunset Boulevard by two straight links of 100 ft and
     92 ft, marked `[assumed]`.
  3. **Cross plazas.** Each pedestrian area gets a centre node with straight links to every point where a path meets its
     outline, so guests cross a plaza instead of walking around its edge.
  4. **Attach and simplify.** Mapped elements attach at the point nearest a walkway. Several elements, such as a land's
     shops, attach at their centroid, and `at` pins a specific OSM node. Plain stretches of path between junctions merge into
     one edge with the summed length. Junctions closer than `cluster_ft` merge into one.
  5. **Report.** Import notes list dropped pieces with their gap to the network, and any attraction far from a walkway.
- **Inputs, committed and dated:** `osm/wdw-dhs/extract-2026-10-18.json` (ODbL, © OpenStreetMap contributors), the Overpass
  query that produced it, and the mapping file.
- **`data/parks/wdw-dhs.yaml`** (approved, read-only): 187 nodes and 240 walkways, sources in every node's comment.
  - **Rides:**
    - Rock 'n' Roller runs its full model.
    - Eight simple rides use ~90% of fan-listed capacity (Jason's decision). Tower of Terror uses the 1,680/hr forum
      calculation; Alien Swirling Saucers has no listed figure, so it uses TouringPlans' line speed.
    - Durations are ride times; Rise's is the full 18-minute experience including preshows.
  - **Shows:** 7 scheduled shows on the 2026 TouringPlans grids. Fantasmic runs at 8:00 pm on a 9 pm close.
  - **Continuous attractions:** Vacation Fun and Walt Disney Presents run as simple rides.
  - **Other nodes:** 3 character meets, 18 dining locations, 5 shop groups and a small `elsewhere`.
  - **Attendance:** low / medium / high are 18,500 / 28,300 / 38,000 (medium is the TEA 2024 daily average). The scenarios are
    `dhs-{low,medium,high}-day`.
- **New park features** (all opt-in, so the synthetic park's results are unchanged):
  - **Node kinds:**
    - `junction` is a plain walkway node.
    - `shop` is a visit at a place, like `elsewhere` but never the Lightning Lane sink.
    - `meet` sees one party at a time for `meet_s` each. A simple ride can't seat a party of 6 per 60 s vehicle at ~100
      guests an hour.
  - **Lightning Lane tiers** (Jason's decision). Nodes declare `tier1`, `tier2` or `single_pass`.
    - Multi Pass buyers book `advance_by_tier` (1 Tier 1 + 2 Tier 2) before the day, then rebook any Multi Pass ride after
      each use or lapse.
    - `single_pass_share` of parties also buy the Rise Single Pass: one booking, never rebooked.
    - Single Pass draws come from their own stream.
  - **Lightning Lane on simple rides.** Each vehicle boards Lightning Lane first, up to 2/3 of its seats, so standby always
    moves. The posted standby wait allows for the seats Lightning Lane takes.
  - **Walkway fixes** (Jason's decision, after the first Hollywood Studios run gridlocked):
    - `congestion.min_capacity_length_ft` (100 ft here): capacity and jam use at least this length, so a short link no longer
      collapses. A 10 ft link held 15 guests.
    - `decision.reroute_s` (300 s here): routes are recomputed from current walking times, so guests detour around jams.
  - **Logging and targets:**
    - Simple rides and meets log their posted waits at each board refresh, so park reports include them.
    - `calibration_targets.rides` holds targets per node; Everest and Haunted Mansion keep their legacy fields.
- **Control room:** marks and labels scale with the map, junctions get no marker, and a big park labels rides, shows and the
  entrance (other names show on hover).
- **Synthetic park checked:** its medium-day log is byte-identical except for three new kinds of line or field. These are
  posted-wait lines for its two filler rides, and ` shop=0` / ` meet=0` in snapshots.

## Why the first run gridlocked
On the first medium day guests spent 79% of their time waiting to get onto jammed walkways.
- **Short links held too few people.** Capacity is length × width × comfortable density. The importer's 10 ft links from an
  attraction to its path held 15 guests and jammed at 45. At rope drop the travel-time formula slowed a crossing about
  40-fold, flow collapsed below the ride's capacity, and the jam spread.
- **Fixed routes funnel everyone into narrow pieces.** Once the links were longer, 64% of time was still spent waiting,
  because every guest took the same shortest route. On it lay a 56 ft flight of 10 ft-wide steps at the Animation Courtyard
  entrance; 10 ft is the default `[assumed]` width for steps.

The synthetic park never showed either problem: its shortest edge is 150 ft and every edge is a wide main path. With the
capacity floor and rerouting, walkway waiting is 0.0–0.3% of guests' time. What remains are short blocks as show audiences let
out, for example Indiana Jones' 2,150 seats emptying onto one path.

## Calibration (medium, 10 seeds, failures on; approved, docs/tuning-log.md)
Changes, all `[assumed]` and in this park file only:
- patience 40 ± 15 → 75 ± 30;
- wait penalty 0.03 → 0.012 per minute;
- ride and meet appeals fitted to Queue-Times over six rounds.

Some appeals exceed 1.0. Rock 'n' Roller's thrill mismatch with the average guest (1.0 against a mean tolerance of 0.55)
costs it about 0.27 utility, which its appeal of 1.31 offsets.

| Posted avg / busiest hour (min) | Queue-Times 2026 [fan] avg / avg daily max | before | after |
|---|---|---|---|
| Slinky Dog Dash | 63 / 83 | 25.5 / 35.2 | 60.2 / 76.6 |
| Rise of the Resistance | 47 / 65 | 24.5 / 32.4 | 45.1 / 60.3 |
| Rock 'n' Roller Coaster | 45 / 63 | 12.4 / 17.6 | 43.6 / 58.5 |
| Runaway Railway | 34 / 45 | 13.7 / 18.1 | 31.6 / 48.2 |
| Tower of Terror | 30 / 53 | 11.1 / 16.9 | 28.2 / 43.5 |
| Toy Story Mania | 30 / 51 | 10.6 / 15.3 | 27.8 / 43.2 |
| Smugglers Run | 29 / 47 | 15.1 / 18.8 | 27.6 / 40.9 |
| Alien Swirling Saucers | 16 / 30 | 16.0 / 21.2 | 15.4 / 24.5 |
| Star Tours | 7 / 13 | 9.9 / 13.6 | 6.5 / 16.4 |
| Meets: Mickey & Minnie / Olaf / Edna (avg) | 31 / 15 / 14 | 23 / 18 / 10 | 29 / 15 / 14 |

Notes on the calibration:
- **Averages:** every target average lands 0.5–2.8 min below Queue-Times.
- **Peaks:** the busiest-hour mean sits below Queue-Times' average daily maximum for most rides, as expected, since a day's
  maximum runs above its busiest hour.
- **Vacation Fun is not fitted.** Queue-Times shows a flat 10/10 because Disney posts a fixed wait for the continuous film; the
  sim gives about 1 min.

Crowd levels (5 seeds each, Slinky / Rise / Runaway Railway averages): low 43 / 27 / 7, medium 60 / 45 / 32, high 71 / 58 / 47.

## Known gaps and next steps
- **Headliner models.** Slinky Dog Dash is next: it fits `multi_train_coaster` (4 trains × 18 riders, 2 launches [wiki];
  track length is missing from the research). After it: Tower of Terror, Rise of the Resistance, then the trackless and
  simulator rides.
- **Turn-aways at the entrance.** At 60-minute waits, guests whose patience is shorter turn away when they reach the ride:
  about 4,900 a day at Slinky. Guests choose on a 5-minute-old board without considering their own patience. M7 tried
  patience-aware choice and it had no effect.
- **Low days.** Mid-tier rides look short against TouringPlans' crowd-level ranges, which are fan data and not a target here:
  Runaway Railway 7 min against 36–65, Toy Story Mania 7 against 19–68.
- **Lightning Lane share** runs 10–25% by ride on a medium day. There is no Hollywood Studios target yet; the M4 band (26%) was
  for the original three rides.
- **Missing places.** These guide-map locations aren't in OSM and aren't placed yet: BaseLine Tap House, Ice Cold Hydraulics,
  Epic Eats, Market, Sunshine Day Bar, the Sunset Boulevard Joffrey's, Oscar's and Jessie's Trading Post. The character meets
  use a nearby building's position.
- **Widths and the links to fix by measuring `[user]`:**
  - every width is a default by path type;
  - chokepoints to measure: the Animation Courtyard steps, the Grand Avenue connector, the Toy Story Land and Galaxy's Edge
    entrances;
  - the two straight links that connect Fantasmic.
- **Data still missing:**
  - theater sizes for Little Mermaid, Disney Jr. and Villains;
  - most restaurant seat counts;
  - Mickey & Minnie and Edna meet times;
  - the current Lightning Lane purchase share;
  - the 2025 TEA attendance figure.
- **Closures:** Vacation Fun closes again from 2026-10-28 (fan lead). Animation Courtyard became part of the "Walt Disney
  Studios Lot" in 2025; the guide map still uses the old name.
- **Control room:** labels overlap where attractions sit close together (Villains and Rock 'n' Roller, Frozen and Vacation
  Fun).

## Code review
A Go review of the whole M9 diff found nothing critical or high. Fixed:
- **Duplicate ways:** a way listed twice in an extract (bare, then tagged) is no longer dropped from the walkway graph.
- **Large Lightning Lane parties:** tiered simple rides must let Lightning Lane seat the largest party within its 2/3 share,
  otherwise such a party could wait behind standby indefinitely.
- **`exclude_inside` references:** a bad or missing reference is now an error, not silently ignored.
- **Tier on a ride without a Lightning Lane line:** `validate` now reports it, instead of the run failing later.

Kept as designed:
- **Shops:** visiting the same shop twice is penalized, like any other activity; only `elsewhere` is exempt.
- **Meet posted wait:** it counts the parties waiting but not the party being met, so it understates by up to one meet. The
  calibration was fitted with this definition.

## Decisions (CLAUDE.md, M9)
- Real park first: Hollywood Studios from the guide map plus OpenStreetMap, with a new `[osm]` provenance tag (ODbL).
- Rides start simple with sourced capacity (90% of listed); headliners get full models one at a time.
- Nodes for every ride, show, meet and dining location; shops grouped per land.
- Importer: committed dated extract plus mapping; it writes a draft once and never overwrites it. The path graph keeps
  junctions and attractions; widths are defaults by path type.
- Lightning Lane tiers plus Rise Single Pass; simple rides get a Lightning Lane line (2/3 cap).
- Attendance 28,300 medium, ±35%.
- Gridlock fix: a 100 ft capacity floor plus rerouting every 5 minutes.
