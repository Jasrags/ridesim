# M10 results: Tower of Terror (2026-10-18)

Scope (Jason): give Tower of Terror a full model, the second headliner after Slinky Dog Dash. Its ride system does not fit the
coaster archetype, so M10 adds a new one, `elevator_drop` (spec §7a, `internal/ride/elevator`).

## The ride system (sourced)
- **Layout** [wiki][fan towersecrets][fan wdwmagic 2015]:
  - two mirrored sides;
  - on each, two show shafts (A/B, C/D) lift a cab from the basement one floor to load, then through the hallway to a Fifth
    Dimension scene, where the cab drives into the side's one drop shaft (E, F);
  - after the drops the cab backs out to one of two basement unload doors, then backs into a show shaft again.
- **Cabs:** 8 self-propelled cabs [wiki], 4 per side [fan], 21 seats in 3 rows of 7 [wiki].
- **Capacity:** 1,680/hr is a theoretical maximum from a WDWMagic forum post [fan]: each side dispatches every 90 s, so 2 sides
  x 21 seats x 40 = 1,680. No official figure was found.
- **Preshow** [user]: two library rooms off the lobby that open into the boiler room, holding about as many as Haunted
  Mansion's stretching rooms. Group size (60), length (2 min) and turnover are `[assumed]`.

## The model
- **Places:** each show shaft, drop shaft and unload door holds one cab.
- **Waiting:** a cab that finishes a step and finds the next place taken waits where it is (logged as a hold), holding its
  current place. Waiters are served first come, first served.
- **No gridlock:** validation keeps cabs per side below the number of places (4 < 2 + 1 + 2).
- **Seating:** the coaster's row rule. Parties share the 7-seat benches, so seats are modeled one by one `[assumed]`.
- **Shared parts:** queues, Lightning Lane merge and preshow are the same code every ride type uses.
- **Stops:** a ride stop freezes every cab with its remaining time. The new `side_down` failure freezes one side, so capacity
  halves and the ride stays open. It is logged as `side_down` / `side_up`, not as a ride stop.
- **Cycle times** are all `[assumed]`, since no segment timings were found. They are fitted to 90 s per side with the drop
  shaft as the bottleneck:

  | Resource (per side) | Busy per cab | Parallel | Per-side interval |
  |---|---|---|---|
  | drop shaft | 90 s | 1 | **90 s** |
  | show shafts | 12 + 8 + 47 + 110 = 177 s | 2 | 88.5 s |
  | cabs (lap) | 321 s | 4 | 80 s |
  | unload doors | 14 + 40 = 54 s | 2 | 27 s |

  Show, drop and the drive to the door sum to the 3:34 ride time [wiki].

## Checks
- **Fixed times:** 1,680/hr, one dispatch every 45 s (90 s per side); the preshow runs only full shows.
- **One side only:** 840/hr.
- **Random times, park party mix, Lightning Lane, all failures:** about 1,540/hr; same seed gives the same log.
- **Model unit tests:**
  - a place never holds two cabs;
  - a ride stop freezes everything;
  - a side outage halves capacity and the down side carries nobody;
  - a fleet that could gridlock is rejected.
- **Validation:** every new error-level check has a broken fixture.
- **Synthetic park:** its medium-day log is byte-identical to the previous commit.

## Hollywood Studios (medium, 10 seeds, failures on)
Tower of Terror carries ~1,520/hr in the park (10-seed mean), against ~1,600/hr as a simple ride.

| Posted avg / busiest hour (min) | Queue-Times 2026 | simple ride | full model |
|---|---|---|---|
| Tower of Terror | 30 / 53 | 28.8 / 44.0 | 31.3 / 49.0 |

- **No re-fit:** the average lands 1.3 min above target (the other rides sit 0.5–3 min below), and the busiest hour moved closer.
  Appeal stays 1.07.
- **Other rides** moved by at most 0.9 min. Slinky went 60.8 → 59.9, probably because guests rebalance toward the busier
  Tower of Terror.

## Code review
A Go review of the new code found nothing critical or high. Fixed:
- **Re-entrant boarding (medium).** Seating a party can drain the preshow, which boards again inside the same pass, so a cab's
  restraint check could start twice. The second start drew new random times and replaced the first timer. The check now starts
  once per load, and both boarding paths skip a cab that has already left load. The 10-seed results are unchanged, so the case did
  not occur in these runs. A liveness test with a preshow checks that every cab keeps dispatching.
- **Startup holds** named no place; cabs now start at `BASEMENT`.
- **Side names** must be distinct single words, because they become place ids.

## Open
- **`side_down` is off in the DHS scenarios.** Its rate (0.1/hr, 30 min) is a guess, and the "capacity halves" behavior is an
  unverified lead. Studies and ops can turn it on.
- **Cycle times need observing `[user]`:** load and unload dwell, the drop sequence length, and the time between dispatches on one side.
- **Preshow:** the 60-guest group, 2-minute show and where Lightning Lane merges (before the library, `[assumed]`).
- **Not modeled:** cabs crossing sides over the basement bridge, and the maintenance bay that holds a spare cab.
- **`ridesim run -ride`** still runs coasters only. Tower of Terror runs in park days and tests, as Haunted Mansion does.
