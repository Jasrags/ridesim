# M12 results: Millennium Falcon: Smugglers Run (2026-10-18)

Scope (Jason): give Smugglers Run a full model. Its ride system is four turntables of cockpit simulators, so M12 extends the
continuous_load archetype (Jason's choices: extend continuous load; capacity emerges from the sourced 45 s rotation).

## The ride (sourced)
- **Ride system:** four independent turntables of 7 cockpits. Each "rotates approximately every 45 seconds" [wiki]. If one
  turntable goes down, the other three continue [fan WDWNT].
- **Crew:** 6 per cockpit: 2 pilots, 2 gunners, 2 engineers [wiki].
- **Length:** the flight is about 4.5 min [fan TouringPlans]; darkridedatabase gives 5 min.
- **Capacity:** ~1,800/hr is Disney's figure (via a NYT reporter) [fan]. Full cockpits at 45 s would give 4 x 6 x 80 = 1,920/hr.
- **Before boarding:** one Hondo preshow room (about 4 min with the chess room), then one of two chess rooms, then six loading
  stations [fan TouringPlans].
- **Crews and single riders:** crews are formed by color card [fan]. Single riders skip the preshow and often become engineers [fan].
- **Jason's observation `[user]`, one sighting:** the grouper fills the first four positions (pilots, gunners) from Lightning Lane
  and standby, and the engineer seats with others.

## What was built
- **Chains (`conveyor.chains`):** several identical independent loops, each with its own load point and clock. They start
  staggered, so with four chains a cockpit reaches a door every 11.25 s. Haunted Mansion is one chain and is unchanged
  (byte-identical logs).
- **Shared vehicles (`party_vehicle_policy: shared`):**
  1. Parties fill the first `party_seats` seats. A party larger than that boards only an empty cockpit.
  2. Single riders fill the rest.
  3. Parties fill anything left.
  4. When the next party doesn't fit, the grouper calls one that does from up to `callout_parties` back (10, `[assumed]`, as
     Tower of Terror's callouts).
  5. A party larger than a vehicle still takes consecutive vehicles.
- **One turntable out (`side_down`):** freezes one chain, including its cockpits in flight; the others keep running. Not enabled in
  the DHS day scenarios, since its rate is `[assumed]`.
- **Stages for continuous rides**, and the **single-rider preshow bypass**: the config field existed, but no ride had used it before.
- **Smugglers ride file** `data/rides/wdw-dhs-smugglers-run.yaml`:
  - Hondo room: 1 room x 84, 2 min.
  - Chess rooms: 2 x 42, 90 s.
  - Load area: 36 (six loading stations).
  - Route per cockpit: 15 s board, 270 s flight, 15 s unload, 15 s back = 7 x 45 s.
  - Group sizes, the Hondo/chess split of TouringPlans' 4 min, and the dwell times are `[assumed]`.

## Checks
- **Solo guests, every seat filled:** 1,920/hr, a cockpit every 11.25 s.
- **Park party mix, demand above capacity:**

  | Grouping rule | Riders/hr | Empty seats per cockpit |
  |---|---|---|
  | as modeled | 1,797 | 0.39 |
  | without the grouper's callouts | 1,551 | |
  | without the front-four rule | 1,846 | |

  The modeled figure is Disney's ~1,800.
- **One of two chains down:** half the riders, and nothing dispatches from the down chain.
- **Full flow with every failure kind:** same seed, same log; every guest who joined boarded, reneged or is still waiting.

## Hollywood Studios (medium, 10 seeds)
Smugglers carries ~1,720/hr in the park, against ~1,542 as a simple ride. At appeal 0.90 it posted 24.6 / 38.4 min. Re-fit to 0.96
(approved): **28.0 / 41.8 min** against Queue-Times' 29 / 47. About 1,650 guests a day turn away at its entrance. Other rides moved
within seed noise.

## Code review
A Go review found one medium issue, now fixed. Single riders who skip the preshow could take the four front seats ahead of parties,
because the boarding order merges them in by arrival. Party seats now take parties only (`lines.HeadParty`). A unit test covers this;
the effect had been small, 3 of 961 cockpits in a test day. Also fixed:
- **Staggered start:** the start uses each chain's own clock, so a stop or outage before a chain's first cockpit delays it too.
- **Load area:** single riders who skip the preshow no longer count against the load area the stages fill.
- **Validation:** `side_down` on a single-chain ride or without a duration is an error, and so is the bypass without a single-rider line.

Noted, not changed:
- A party of 7 or more caught by a turntable outage mid-boarding waits for that turntable.
- Grouper callouts skip the Lightning Lane queue on rides without a preshow, which Smugglers isn't.

## Open
- **Grouping rule:** the front-four rule is from one observation, and the callout reach is a guess.
- **Brief stops:** the DHS scenarios list `brief_stop` but not `chain_stop`, so continuous rides there get only long downtime.
- **Validation warning:** the loop-time check warns for Smugglers. It compares the cockpit loop with an experience time that
  includes the preshow, so it is informational here.
- **Not modeled:** the six loading stations as separate doors, and stopping a single wedge.
