# M14 results: Toy Story Mania, and every Hollywood Studios ride modeled (2026-10-18)

## Toy Story Mania
**Sourced:**
- three tracks, with tracks 1 and 2 sharing a station and the third separate [fan darkridedatabase]; the third opened in 2016 [wiki];
- trams of two linked cars with back-to-back benches of two, so 8 riders [wiki][fan MousePlanet];
- trams stop at each game under a central controller [fan WDWNT];
- 2,400/hr theoretical and 2,160 operational [fan crooksinwdw].

**Model (Jason):** the M12 continuous-load chains, with no new ride code.
- **Tracks:** three chains, each bringing a tram to its load point every 40 s, which gives 2,160/hr (the 90% rule).
- **Seating:** parties share trams, and callouts fill gaps. The callout behavior is from an unverified lead ("party of two"); its
  reach is `[assumed]`.
- **Assumed:** the route (20 s load, 300 s of games, 20 s unload, 60 s back) and 10 trams per track. The game stops sit inside the ride
  time.

**One validation change:** the continuous capacity ceiling now checks the build target (`theoretical_riders_per_hr_at_target`) when
one is set, else the listed figure. A ride built to 90% of its listed capacity would otherwise always fail.

**Checks:** solo guests give 2,160/hr, a tram every 13.3 s across the three tracks. The park party mix gives ~2,090/hr, with 0.26
empty seats a tram.

**Medium day:** ~2,080/hr in the park, posting **28.1 / 43.6 min** against Queue-Times' 30 / 51 (28.0 / 43.4 as a simple ride). No
re-fit.

## Hollywood Studios: where every ride stands (medium day, 10 seeds, posted avg / busiest hour, min)
| Ride | Model | Queue-Times 2026 | Sim |
|---|---|---|---|
| Slinky Dog Dash | coaster, 4 trains (M9 follow-up) | 63 / 83 | 61.4 / 87.7 |
| Rise of the Resistance | stage chain + vehicle pairs (M11) | 47 / 65 | 46.8 / 60.0 |
| Rock 'n' Roller Coaster | coaster with preshow (M3) | 45 / 63 | 44.0 / 58.1 |
| Runaway Railway | preshow + trackless trains (M11 follow-up) | 34 / 45 | 33.6 / 50.6 |
| Tower of Terror | elevator_drop (M10) | 30 / 53 | 29.5 / 47.1 |
| Toy Story Mania | 3 track chains (M14) | 30 / 51 | 28.1 / 43.6 |
| Smugglers Run | 4 turntable chains, shared crews (M12) | 29 / 47 | 27.8 / 40.8 |
| Alien Swirling Saucers | simple ride | 16 / 30 | 16.3 / 25.0 |
| Star Tours | simulator_bays (M13) | 7 / 13 | 7.0 / 17.6 |

Every attraction with a Queue-Times target lands within 2 min of its average. Busiest hours mostly sit below the "average daily
maximum", as expected, since a day's maximum is higher than its busiest hourly mean. Alien Swirling Saucers is the only ride without a
full model (a small spinner).

## Open
- **Toy Story Mania:** tram count, spacing and game timing need observing, and the shared station for tracks 1 and 2 is not modeled.
- **Brief stops:** they still never run on continuous rides in the DHS days (Smugglers, Toy Story Mania), because the scenarios list
  `brief_stop`, not `chain_stop`.

## Fixes pass (2026-10-18, Jason's choices)
- **Brief stops on continuous rides:** park runs now treat a scenario's `brief_stop` as `chain_stop` for continuous rides (unless it
  lists `chain_stop` itself, so the synthetic park is byte-identical). Smugglers Run and Toy Story Mania get brief stops in the DHS
  days.
- **Rise opens late:** new failure kind `opening_delay`, drawn once at opening: 25% of days [fan TouringPlans], 45 +- 30 min
  `[assumed]`. While delayed the ride shows closed, guests turn away, and posted stats skip it as Queue-Times does.
  - Rise's long stops were trimmed 19 -> 16 min so whole-day downtime stays ~11%; a test measures that from opening.
  - Rise's appeal was re-fit 1.02 -> 1.06 (approved): **48.2 / 65.7 min** against 47 / 65.
- **Kept as is (Jason):** one-side / one-bay outages stay off until there are rates; rooms keep draining in room order.

Medium day after the fixes (10 seeds, posted avg / busiest hour): Rise 48.2 / 65.7, Smugglers 29.2 / 42.6, Toy Story Mania 28.6 /
45.2, Slinky 61.3 / 90.2; the rest within seed noise of the table above.

