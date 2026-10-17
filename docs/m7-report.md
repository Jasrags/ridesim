# M7 results (2026-10-17)

Scope (spec §9): the park-level crowd model on the synthetic park. It covers the graph, party agents, choice on
stale posted waits, walking with congestion and spillback, show-exit surges, and Lightning Lane return windows.
Park attendance and passes replace the scripted arrival curves and LL demand.

## What was built
- **One simulation, many rides.** `Sim.Scope(node)` stamps each event with `src` and gives every ride its own RNG
  streams (`everest/failures`, ...). Each ride's randomness is independent of its neighbours, so paired studies
  stay paired. Events without a scope keep the old byte format, and every earlier log is unchanged.
- **Party exits.** Rides tell a party when it is off the vehicle (after unload, or the last buggy of a large
  party), or when it balks or reneges. The park defers the callback to a new event, so a ride is never re-entered.
- **`internal/park`**
  - **Graph:** static free-flow shortest routes; ties break in file order.
  - **`TravelTime`:** `fixed_delay`, or BPR `congestion` with C = length x width x comfortable density.
    - Jam density is a hard cap, but an empty edge always admits one party, so the park cannot deadlock.
    - Parties wait FIFO at a jammed edge.
    - A ride queue past its physical capacity takes walkway width (spillback), never more than 90% of it.
  - **Activities**
    - Hosted ride models: the full coaster and omnimover.
    - `ride_simple`: one vehicle per minute, capacity spread over it, parties never split.
    - Scheduled shows: the audience leaves by `exit_edge` and decides at its far end.
    - Dining: FIFO for seats.
    - `elsewhere`: the rest of the park, with unlimited capacity and a sampled visit length.
  - **Agents:** the whole day's parties are drawn at open, from attendance x hourly share per hour.
    - Traits from the park file: party size, patience, thrill tolerance, stay length, Lightning Lane purchase.
    - Choice is a softmax over the top k by utility, using the posted-wait board refreshed every 300 s.
    - Utility terms: appeal, thrill match, wait, walk x (1 + fatigue), repeat, LL window bonus, meal-window bonus.
  - **Lightning Lane** ([user] design):
    - Purchasing parties book `advance_bookings` before the day, in random order, each the earliest window with
      inventory at or after their arrival.
    - After that, one more booking each time a pass is used or lapses.
    - Each ride is booked at most once a day, and each ride sells a share of its listed capacity per window.
  - **Closures** (Jason's decision): during long downtime the board shows the ride closed. Guests don't choose it,
    and anyone who arrives anyway turns away.
- **Park scenarios** (`park_id` with `crowd_level` or `attendance`, optional `walking_mode` and
  `no_lightning_lane`). The new files are `data/scenarios/park-{low,medium,high}-day.yaml`. Park studies name a
  `report_node`.
- **Park report** (`run -scenario` and `report -log` detect park logs):
  - per-node ride reports;
  - arrivals, departures, and in-park peak and mean;
  - time share by state;
  - LL booked / redeemed / lapsed;
  - per-edge peak, mean n/C, time over C and blocked parties;
  - joins and posted waits around each show exit;
  - calibration against the Queue-Times targets.

  Ride reports gain `PostedMean` / `PostedPeak` (busiest clock hour), with long downtime plus 15 min recovery
  skipped. Study tables gain `posted_mean_min` and `posted_peak_hr_min`.
- **Speed:** a 40,000-guest, 13-hour park day runs in about 1.5 s and writes about 90 MB of events.

## Calibration (medium crowd, Jason's decision; 10 seeds, failures on, congestion walking)
Approved `[assumed]` park-file changes are in `docs/tuning-log.md`:
- elsewhere visits 30 +/- 15 min;
- LL `elsewhere_share` 0.20, `inventory_share` 0.40, `purchase_share` 0.30.

| | Everest posted avg / peak hour | HM posted avg / peak hour |
|---|---|---|
| target [fan] (Queue-Times) | 19 / 35 min | 26 / 42 min |
| sim, medium (10 seeds) | 23.4 / 46.1 | 29.2 / 56.0 |
| sim, medium without seed 6 (cascade storm, below) | 21.2 / 35.6 | 29.3 / 57.5 |
| sim, low / high | 9.2 / 30.0, 28.4 / 44.7 | 12.9 / 22.3, 39.0 / 59.1 |
| before calibration (medium, 3 seeds) | 16.3 / 23.7 | 20.8 / 26.4 |

| Realized LL share (Everest / RnR / HM) | low | medium | high |
|---|---|---|---|
| band [fan] | 13% | 26% | 33% |
| sim | 12.4 / 16.4 / 14.3% | 23.2 / 29.1 / 26.1% | 30.1 / 33.7 / 37.0% |

- Numbers are after the code-review fixes below, which change random draws. Without the seed-6 outlier, averages run
  8-12% above target and Everest's peak hour is on target. Jason chose to accept that over tuning into seed noise.
- **Cascade storms after a stop (seed 6).** A brief stop leaves Everest's trains stacked. Each restart holds a train
  at a course brake for more than 5 s, so it cascades again (45 s reset): 72 cascades in 2 h, a posted wait of
  556 min, and 1,522 riders/hr for the day. It is the same coaster-model mechanism as the startup burst below. It
  hits about 1 park day in 10, and is rare in ride-only runs.
- **HM's peak hour is 20% high.** Chain stops and accessibility holds make `queue_len_over_throughput` spike,
  because it divides the queue by the last 15 minutes' throughput. Daily posted maxima on HM reach 80-200 min.
  This is a posted-algorithm effect from M2, not a park effect.
- **Things tried and dropped:**
  - giving `elsewhere` a wait that follows the rides' waits: the right effect on peaks, but it pushed so many
    guests to dining that `hub_dining` gridlocked;
  - having guests skip options whose wait exceeds their patience: no measurable effect.
- **Code-review fixes:**
  - The single-rider roll and Lightning Lane bookings now draw from their own streams (per party for LL), a fixed
    number of draws each, so variants stay paired.
  - Validation rejects a `ride_simple` vehicle or a dining room that cannot seat the largest party (it would block
    the line forever).
  - Parties that go home drop their passes and no longer count as lapses.
- **Bug found during calibration and fixed** (9547a30): every leave from a jammed edge retried every waiter and
  logged another `edge_blocked`. One jam produced 2.7 million events and a 2 GB log. The baseline never jammed.

## Behaviours (spec §9)
- **Show-exit surge (theater -> Haunted Mansion)**, 10 seeds:
  - HM's posted wait rises within 30 min after 39 of 40 shows (about 21 -> 70 min after the first two).
  - Joins in the 15 min after the 10:30 and 12:30 shows rise 32% and 114%.
  - After the 14:30 and 16:30 shows joins are flat or lower: the park is already busy and the posted wait is high.
  - The narrow `theater_haunted_mansion` link runs over capacity for several minutes after each show.
- **Lightning Lane return clustering:**
  - 16.5% of LL guests arrive in the first 5 minutes of their 60-minute window, about twice an even spread.
  - Arrivals then decay steadily to 4% in the last 5 minutes.
  - About 16% of bookings lapse at medium (19% low, 13% high).
- **Breakdown ripple: weak.** Same seed with and without long downtime (common random numbers), 7 Everest closures
  plus 30 min after each:
  - the flat ride gains 5%;
  - boat and dining are flat;
  - RnR and HM are slightly down.

  Most displaced demand goes to `elsewhere`, which has unlimited capacity. A stronger ripple needs the sink to
  feel crowding, which is the same idea that gridlocked dining above. Left for a later milestone.
- **Spillback:** tested (an overflowing queue narrows the walkway and slows crossings). On the calibrated medium
  day, ride approach edges stay well below C (mean n/C under 0.1), so spillback rarely binds.

## Explained differences
- **Everest ~1,830 riders/hr vs ~2,012 in M2.** M2 used single guests. With the park's party mix about 3.5 of 34
  seats ride empty per dispatch. The ride-only baseline gives 1,828/hr with the mix and about 2,034/hr with single
  guests (3 seeds each). M4 already reported 1,830 at medium. This is not a park effect.
- **Startup cascade burst (coaster model).**
  - Trains start bunched behind the station, and the first load takes the full fill wait.
  - Train 4 waits at the Yeti brake for more than 5 s, which is a cascade with a 45 s operator reset, repeated
    30-47 times in the first hour.
  - Warmup hides it from metrics, but in park runs rope-drop guests see Everest at about half capacity until it
    clears.
  - **Recommended follow-up** (also fixes the mid-day storms above): treat holds while a stacked ride recovers
    from a stop, or from opening, as planned and not cascades. For example, arm cascade detection again only after
    each train has completed one lap since the last resume. It changes coaster behaviour, so it needs Jason's call.

## Decisions (CLAUDE.md, M7)
- `elsewhere` sink for the rest of the park.
- Lightning Lane: advance booking plus rebook after use or lapse; inventory per window.
- Calibration may tune park-file `[assumed]` values only, against the medium crowd.
- Long downtime closes a ride on the board; posted stats skip closure plus 15 min recovery.
- Scoped sims: event `src`, per-scope streams.
- Fatigue raises the walk penalty only. Departure follows `stay_hours`; the park-file comment was corrected, and
  fatigue-driven departure is a later refinement.

## Known gaps and next data
- The `elsewhere` sink absorbs breakdown ripple. A crowding-aware sink, or more modeled attractions, would
  strengthen it.
- `ll_passes_per_guest` is read as bookings per purchasing party per day. A real Multi Pass day is bounded by
  rebooking, not a fixed count.
- No "book again 2 hours after park open" rule. LL bookings for a closed ride lapse and rebook; there is no
  multi-experience pass.
- Theater: guests choose a show only if they arrive before it starts. There is no standby line for a full house;
  they balk.
- Park runs end at closing time. Guests still in line are not drained, about 13k guests remain at close.
- Log size: about 90 MB per park day. A 20-seed park study holds roughly 2 GB of events at once at full
  parallelism; use `-parallel` to bound it.
- Data still to collect: real per-hour Queue-Times curves for the shape of the day; LL purchase rate and
  inventory; how often guests leave the line for a closed ride.
