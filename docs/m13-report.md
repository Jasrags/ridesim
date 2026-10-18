# M13 results: Star Tours (2026-10-18)

Scope (Jason): give Star Tours a full model in which each simulator bay runs its own cycle, launching when its boarding is done.
That needs a small new archetype, `simulator_bays` (`internal/ride/bays`), and a shared seating package.

## The ride (sourced)
- **Bays:** six simulator bays at Hollywood Studios, 40 riders each in 5 rows [wiki]. The rows hold 8 [fan TouringPlans]; ResortsGal
  says 7, which does not fit 40 in 5 rows.
- **Length:** the flight is 4:30 [wiki]; the whole attraction takes about 7 min [fan].
- **Capacity:** 1,800/hr theoretical and 1,620 operational [fan crooksinwdw]; ~1,500 [fan disneyblog].
- **Boarding** [fan ResortsGal]: cast send guests left or right and assign rows at the gate. When the doors open, the previous riders
  exit and the rows clear, then the next group sits. A safety video plays and cast check each row's strap.

## What was built
- **`simulator_bays`:** each bay repeats a cycle, with no clock tying the bays together:
  1. riders exit;
  2. the next group boards for `load_s`, then waits up to `max_wait_after_ready_s` unless full or no one in line fits;
  3. restraint check;
  4. safety video and flight.

  Bays board from one shared line, the bay that opened first served first. A ride stop freezes every bay; `side_down` freezes one.
- **`internal/ride/seats`:** the row rule and the boarding-group rule with callouts, moved out of the elevator model so both ride types
  share them. Tower of Terror's logs are byte-identical after the move.
- **Star Tours ride file:**
  - 6 bays of 5 rows x 8, parties assigned to rows with callouts (reach `[assumed]`).
  - Cycle 60 exit + 120 boarding + 23.3 check + 60 video + 270 flight = 533.3 s, which gives 1,620/hr (Jason's 90% rule). Only the
    flight is sourced; the rest is `[assumed]` to fit.

## Checks
- **Fixture bays:** they hit their cycle with fixed times; a bay out carries nothing while the other keeps flying; a ride stop freezes
  both; a repeated restraint check is a no-op; same seed, same log.
- **Star Tours:**
  - solo guests give 1,620/hr;
  - the park party mix gives ~1,620/hr, with 0.21 empty seats a flight;
  - the full flow with every failure kind is deterministic and loses nobody.

## Hollywood Studios (medium, 10 seeds)
Star Tours carries ~1,360/hr: demand, not capacity, sets it at a 7-minute ride. It posts **6.9 / 17.5 min** against Queue-Times'
7 / 13. Appeal is unchanged at 0.50, and other rides moved within seed noise.

## Code review
A Go review found nothing critical, high or medium. It confirmed that the seating move changes nothing for Tower of Terror, that a bay
cannot get stuck or lose riders, and that every switch on ride type covers the new kind. Fixed: a negative spread on `side_down`'s
duration is now rejected. Noted: all six bays open at once and stay in step for a while at fixed times, a start-up transient only.

## Open
- **Assumed times:** the cycle split (boarding 2 min, exit 1 min, video 1 min) needs timing `[user]`.
- **Gates:** each gate's waiting group is not a separate holding area. Guests board from one line as a bay opens, so a bay never waits
  with a pre-formed group ready.
- **Failures:** they are the usual placeholders, since no downtime data was found.
