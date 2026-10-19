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
