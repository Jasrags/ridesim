# M17 results: EPCOT (2026-10-19)

The third park, built the Hollywood Studios way (Jason). Sources:
- the 2026 guide map (Jason's image: names, lands, map numbers);
- an OSM extract from 2026-10-19 (`osm/wdw-epcot/`, ODbL);
- research from 2026-10-19 (ride capacities, LL tiers, shows, hours, Queue-Times targets).

Every ride starts as a simple ride; full models come one at a time.

## New in the park model
- **Node hours** (`hours: "HH:MM-HH:MM"`): an activity can open after the park or close before it.
  - Before it opens, guests don't choose it. Modeled rides stop with cause `area_hours`, simple rides and meets log that cause, and
    posted-wait stats skip the closed time.
  - After it closes, it takes no new guests; anyone already in line still rides.
- **Several gates**: each entrance has an `arrival_share` and an `exit`. A party draws its gate (stream `gate`, drawn only in
  multi-gate parks) and leaves the way it came in.

Hollywood Studios and synthetic-park logs are byte-identical with both features.

**Correction to the kickoff:** World Showcase as a whole opens with the park. Only its restaurants, shops and the two Circle-Vision
films open around 11:00 [fan Ziggy 2026], so node hours are set on those. Frozen Ever After and Remy open at 9:00 [fan ResortsGal].

## The park
- **Walkways:** 440 nodes (355 junctions) and 606 edges from OSM.
- **Gates:** the main entrance (80%) and the International Gateway (20%, `[assumed]`, Jason). No source gives the share.
- **Rides:** 18, as simple rides.
  - Capacity is crooksinwdw's operational figure, or Wikipedia's figure x 90% where crooksinwdw has none (Guardians, Remy, Gran Fiesta).
  - Test Track 3.0 (2025) has no published figure, so it keeps the 2019 Test Track number.
- **Shows:** The American Adventure (1,024 seats, the 2026-10-19 schedule), America Gardens, Palais du Cinéma.
- **Meets:** Mickey & Friends, and Anna & Elsa (~30 s a party, from TouringPlans' line speed over several rooms).
- **Dining:** 43 restaurants. Seats come from WDWMagic's Low / Medium / High bands, except Space 220 (>350 seats [wiki]).
- **Shops:** 15 nodes, one per land or pavilion.
- **Attendance:** 33,200 on a medium day (TEA 2024: 12.13M / 365); low and high are +-35%.
- **Lightning Lane:**
  - Tier 1: Test Track, Remy, Frozen.
  - Tier 2: eight attractions.
  - Single Pass: Guardians.
- **Not modeled yet:**
  - Luminous: one show node would send every viewer to one point on the lagoon.
  - The Food & Wine Festival (Jason).
  - Friendship boats, and Play Zone.
  - Places OSM lacks: Geo-82, Connections, Choza de Margarita, three Joffrey's kiosks, Club Cool, Art of Disney.

## Calibration (approved)
14 ride and meet appeals were fitted to Queue-Times 2026 averages: 9 rounds of 5 seeds, with the M9 rule. The values are in
tuning-log.md.

Medium day, 10 seeds (posted average / busiest hour, min):

| Attraction | Queue-Times 2026 | Sim | Low | High |
|---|---|---|---|---|
| Guardians | 69 / 89 | 68.0 / 89.6 | 46.3 | 85.9 |
| Test Track | 58 / 77 | 57.2 / 74.8 | 38.5 | 66.3 |
| Remy | 46 / 62 | 45.7 / 64.8 | 29.8 | 61.9 |
| Frozen Ever After | 42 / 57 | 40.3 / 53.0 | 26.2 | 48.8 |
| Anna & Elsa | 40 / 75 | 39.4 / 47.5 | 32.6 | 44.3 |
| Mickey & Friends | 22 / 38 | 21.1 / 28.2 | 17.0 | 24.5 |
| Soarin' | 18 / 34 | 17.3 / 29.4 | 6.6 | 28.1 |
| Mission: SPACE | 16 / 21 | 15.2 / 28.2 | 4.6 | 28.9 |
| Turtle Talk | 15 / 18 | 14.7 / 23.7 | 8.1 | 20.1 |
| Living with the Land | 11 / 24 | 10.3 / 20.1 | 3.8 | 18.7 |
| Figment | 10 / 22 | 8.3 / 18.4 | 1.7 | 16.6 |
| Spaceship Earth | 10 / 19 | 9.2 / 19.5 | 1.6 | 18.2 |
| The Seas | 9 / 20 | 7.5 / 16.8 | 1.3 | 15.4 |
| Gran Fiesta Tour | 8 / 16 | 6.0 / 12.9 | 0.9 | 13.8 |

Not fitted: Canada Far and Wide (14 / 14) and the Short Film Festival (10 / 10). Disney posts fixed waits there; the sim gives ~0.

## Gaps and leads
- **Anna & Elsa's busiest hour** is 47.5 min against an average daily max of 75. The meet runs at a steady ~30 s a party; real rooms
  open and close with demand.
- **Lapsed LL passes:** about 5,200 a medium day, 36% of bookings (Hollywood Studios: ~30%). World Showcase distances may make
  return windows harder to keep. Worth a look before EPCOT challenges.
- **Low days** look very short for mid-tier rides (Figment 1.7, Spaceship Earth 1.6). This is the same pattern as Hollywood Studios
  at M9.
- **Long walk-in links:** attractions inside large buildings attach by straight links of 160-330 ft (Living with the Land, Garden
  Grill, Gran Fiesta, Via Napoli, Tutto Gusto, Regal Eagle).
- **Full-model candidates** with existing archetypes:
  - continuous_load: Spaceship Earth and The Seas (omnimovers), plus the boats (Frozen, Living with the Land, Gran Fiesta);
  - simulator_bays: Soarin' (3 theaters x 87) and Mission: SPACE (4 centrifuges x 10 x 4);
  - coaster: Remy, on scene blocks like Runaway Railway, and Guardians (8 trains x 20).

  Test Track likely needs a new archetype.
- **To observe `[user]`:**
  - the International Gateway share;
  - Test Track 3.0 vehicle count and dispatch interval;
  - Anna & Elsa rooms in use;
  - Luminous viewing spots.

## M18 follow-up: Guardians of the Galaxy: Cosmic Rewind (2026-10-19)
- **Model** (Jason's choices): `data/rides/wdw-epcot-guardians-cosmic-rewind.yaml`, a multi_train_coaster on a new **dual-load
  station**. The unload block's `alt_next` names a second load platform that rejoins at the same block. Trains take the first
  free platform, both share the launch interval, and `side_down` closes the second platform (own stream; off in the days).
  Existing rides' logs are byte-identical.
- **Sourced:**
  - 8 trains of 5 cars x 2 rows of 2 = 20 [wiki]. Jason's ride photo confirms 2 x 2 per vehicle [user].
  - 3:20 ride [wiki]; two platforms with a separate unload [rcdb][fan].
  - Preshow chain: holding room -> Welcome Center (~2.5 min) -> Phase Chamber (~3 min), batches of up to 140 [fan]; Single Pass
    merges at the first room [fan].
- **Built to 1,800/hr** (90% of the 2,000 design): 80 s per platform (64 + 12 + 4 `[assumed]`), so one launch every 40 s. Eight
  `[assumed]` blocks, each under 40 s with its brake release. Fixed times give 1,800/hr; 7 trains are lap-bound at 1,600.
- **Seating:** parties share the 2-seat rows (`seats_per_row: 1`), because trains leave full "most of the time" [fan BlogMickey].
  With whole rows per party they averaged 17.8 of 20 seats.
- **In the park:** ~1,555/hr, because of stops (~6% of the day) and timing variance.
- **Re-fit (approved):** appeal 1.49 -> 1.38.

Medium day, 10 seeds:

| Attraction | Queue-Times 2026 | Sim |
|---|---|---|
| Guardians | 69 / 89 | 70.5 / 104.5 |

The busiest hour runs ~15 min high, the same pattern as Slinky. Every other attraction is within 0.5 min of the table above.
- **Open `[user]`:** load dwell per platform, how often the second platform is out, and whether cast seat strangers side by side.

## M19 follow-up: Remy's Ratatouille Adventure (2026-10-19)
- **Model** (Jason's choices): each group of 3 trackless vehicles [fan] runs as one 18-seat train on ten scene blocks, loading on
  the dual-load station (left and right sides [fan]) with a separate unload. Built to 90% of 2,220/hr [wiki] = 1,998/hr: one group
  every 32.4 s, so each side gets 64.8 s.
  - EPCOT's version is 2D since 2025-11-14, so there is no glasses step [fan].
  - The single-rider line is open in the model ([fan] it fills odd seats; its hours are not scripted).
- **Seating `[user]`:** Jason's party of 4 shared a vehicle with a party of 2 split one front, one back, so seats fill one by one.
  Whole rows of 3 left vehicles 76% full, because the guest model has almost no single riders (10 rode all day).
- **Downtime `[assumed]`:** a long stop every 12 h (Jason) plus brief stops, about 7%. Every 6 h gave ~14%. The only sources are
  "#4 for downtime at WDW" and a 39-min average outage in 2021.
- **Medium day, 10 seeds:** ~1,683/hr. With the approved re-fit (appeal 1.34 -> 1.28) it posts 45.9 / 70.1 min against 46 / 62.
  The busiest hour runs high, as with Guardians and Slinky.
- **Open `[user]`:**
  - dwell per load side;
  - how many groups the show holds;
  - when the single-rider line opens;
  - stops per day.

## M20 follow-up: Frozen Ever After (2026-10-19)
- **Model** (Jason's choices): boats on ten blocks with one combined load/unload station [fan Coaster101]. The scene order runs
  through the lift, the backwards dip and the forward 28 ft drop [wiki]. It runs 9 boats [fan, former operator]. Built to 900/hr
  (crooksinwdw operational): one 16-seat boat every 64 s.
- **Seating:** whole rows of 4. The former operator says separate parties are not put in one row [fan WDWMagic 2016]. With the
  park's party mix, boats leave ~68% full.
- **Downtime:** fitted to ~12% a day from TouringPlans' 2023 weekly figures (12-21%) and the first-hour outages (11% of days in
  Feb-Mar 2026). That gives late openings on 15% of days and a 25-min stop every ~4 h.
- **Medium day, 10 seeds:** ~553/hr. With the approved re-fit (appeal 0.91 -> 0.74) it posts 42.4 / 80.7 min against 42 / 57.
  The busiest hour overshoots more than the other headliners: late openings and long stops build the line, and 553/hr empties it
  slowly.
- **Open:**
  - The row rule is the biggest lever. TouringPlans' line speed (~860/hr) suggests fuller boats than the operator's account. A
    `[user]` look at how cast seat a party of 2 would settle it.
  - Boat count `[user]`.

## M21 follow-up: Soarin' (2026-10-19)
- **Model:** simulator_bays, as Star Tours.
  - 3 theaters of 87 [wiki][fan WDWMagic], with gates A/B/C of 27/33/27 as the boarding groups [wiki] and callouts `[assumed]`.
  - Built to crooksinwdw's operational 1,879/hr: a 500 s cycle per theater (45 exit + 90 load + 25 check + 40 pre-flight video +
    300 flight).
  - Only the ~5 min flight and 87 seats are sourced; Soarin' Across America's runtime is not published.
- **Medium day, 10 seeds:** ~1,729/hr. It posts 17.5 / 32.1 min against Queue-Times' 18 / 34 with no re-fit (appeal 0.70).
- **Open `[user]`:** the Across America film length, and the time from doors opening to lift-off.

## M22 follow-up: Mission: SPACE (2026-10-19)
- **Model:** simulator_bays, as Soarin' and Star Tours.
  - 4 centrifuges, each a bay of 10 four-seat capsules [wiki][fan TouringPlans]. Capsules are boarding groups of 4: cast combine
    parties into crews, with callouts `[assumed]`.
  - The 3-min briefing is a preshow [fan TouringPlans]; its rooms and size are `[assumed]`.
  - Built to crooksinwdw's operational 1,440/hr: a 400 s cycle per centrifuge with the 4-min mission [wiki].
  - Orange and Green are one ride. Queue-Times reports one wait, and Disney swaps centrifuges between them by demand [fan].
- **Medium day, 10 seeds:** ~1,343/hr. It posts 15.6 / 29.1 min against 16 / 21 with no re-fit (appeal 0.62). The busiest hour runs
  high, as it did as a simple ride.
- **Open `[user]`:** the current Orange/Green split, and how cast form crews of 4.

## M23 follow-up: Spaceship Earth (2026-10-19)
- **Model** (Jason's choices): continuous_load omnimover, as Haunted Mansion.
  - 4-seat vehicles in 2 rows of 2 [wiki], one party per vehicle (parties of 5-6 take consecutive vehicles).
  - Full vehicles every 6 s from the sourced 2,400/hr theoretical [fan crooksinwdw]. No vehicle count was found, so 165 vehicles
    fill a 990 s loop [derived].
  - Scene order [wiki], times `[assumed]` within the 15-min ride, with the backwards descent the longest.
- **Accessibility `[user]`** (Jason): the chain sometimes only slows, and stops fully for wheelchairs. The model has full stops
  only: 8 an hour of 75 s `[assumed]`, standing for both.
- **Medium day, 10 seeds:** ~1,220/hr. With the approved re-fit (appeal 0.70 -> 0.58) it posts 9.3 / 20.3 min against 10 / 19.
- **Open:**
  - partial slowdowns in the continuous model (code);
  - `[user]` how many stops vs slowdowns an hour;
  - the vehicle count.

## M24 follow-up: The Seas with Nemo & Friends (2026-10-19)
- **Model:** continuous_load omnimover, built the way Jason chose for Spaceship Earth.
  - Clam-mobiles of 2-3 [wiki], one party per vehicle; a party of 4+ takes consecutive vehicles.
  - A vehicle every 3.27 s from the 2,200/hr theoretical at the standard 2 riders [fan TouringPlans]; 92 vehicles [derived].
  - Scene times `[assumed]`, and the same full-stop holds as Spaceship Earth.
- **Medium day, 10 seeds:** ~1,688/hr. It posts 7.2 / 17.5 min against 9 / 20, as it did as a simple ride, with no re-fit (appeal
  0.72).
- **Open `[user]`:** vehicle count, ride time, accessibility stops.

## M25 follow-up: Living with the Land, Gran Fiesta Tour, Figment (2026-10-19)
All three are coaster-archetype models (vehicles on blocks). Parties share benches, rows and cars (Jason; no source on seating).
None needed a re-fit.

| Ride | Vehicles and station | Built to | Medium day (10 seeds) | Queue-Times |
|---|---|---|---|---|
| Living with the Land | boats of 40 (2 barges x 5 rows of 4) [wiki], one dock [fan], 10 boats [fan] | 1,440/hr (one boat / 100 s) | 11.1 / 23.9 min, ~1,009/hr | 11 / 24 |
| Gran Fiesta Tour | boats of 15 (5 rows of 3) [wiki], separate unload dock [fan], 15 boats [derived] | 1,490/hr (one boat / 36.2 s) | 7.8 / 15.8 min, ~1,135/hr | 8 / 16 |
| Figment | 4 cars x 7 seats (3 + 4) [fan], stopped loading, separate unload [fan], 11 trains [derived] | 1,980/hr (one train / 50.9 s) | 9.3 / 20.3 min, ~1,716/hr | 10 / 22 |

- **Living with the Land** now closes at 19:00 (node hours), since it usually closes around 7 pm [fan TouringPlans].
- **Figment's fleet:** the counted 1,980/hr needs ~10.2 trains for the 7.5 min loop. The only fleet figure (32 cars = 8 trains)
  would cap it near 1,550, so the model runs 11 and records the conflict.
- **Assumed:** scene times, station times (fitted to each interval) and failures. None of the three has a downtime rate.
- **Open `[user]`:**
  - boat and train counts in service;
  - whether cast seat separate parties together;
  - Living with the Land's closing time.

**Status:** every EPCOT ride with a Queue-Times target now has a full model except Test Track. That needs a new archetype, and
Jason asked to pause before it.
