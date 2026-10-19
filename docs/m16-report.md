# M16 results: Hollywood Studios realism pass (2026-10-19)

Before another park (Jason): fewer turn-aways, the missing data researched, the missing places added, Vacation Fun removed, and a
full Alien Swirling Saucers model. `[user]` widths and ride timings wait for Jason's next visit (list below).

## Turn-aways
**Cause:** guests chose rides whose posted wait (on a board refreshed every 5 min) was already over their own patience. Walks took
4-5 min and the wait barely moved in that time, so they turned away on arrival. Shows had the same problem: guests walked to
performances that were already full (Little Mermaid 6,852 turned away a day, Frozen 2,028).

**Fix (approved):** two park-file switches, `decision.patience_aware` (skip an option whose board wait is over the party's
patience; Lightning Lane returns and shows are exempt) and `decision.skip_full_shows`. Both default off, so the synthetic park is
unchanged; Hollywood Studios turns both on.

| Medium day, 3 seeds | turn-aways at rides, shows, meets | score | posted waits |
|---|---|---|---|
| before | ~30,000 | 47.5 | — |
| patience only | ~15,700 | 49.0 | within ~1 min |
| full shows only | ~25,000 | 47.9 | unchanged |
| both | ~11,000 | 49.1 | within ~1 min |

## Research (2026-10-19)
Found and used:
- **LL purchase share:** 0.30 kept and re-tagged `[fan]`: Disney Tourist Blog (2026-01-02) estimates "anywhere from 20% to half"
  buy Multi or Single Pass; Chapek said about a third (Feb 2022).
- **Places:** the 8 missing guide-map places, mostly from Disney's own pages and fan sources. Joffrey's on Sunset is in OSM.
- **The Walt Disney Studios** (the former Animation Courtyard): announced 2025-07-22, reopened about 2026-05. The Magic of Disney
  Animation opened there 2026-09-14.
- **Vacation Fun:** closes from 2026-10-28, with no reopening date [fan Inside the Magic].
- **Alien Swirling Saucers:** 2 sides x 11 rocket cars of 2, one party per car, a timed 3-minute cycle [fan yourfirstvisit], tier 2.
  On 2026-09-22 only one side was running [fan Inside the Magic].

Not found (values stay `[assumed]`):
- **Theater seats:** none for the current Little Mermaid, Disney Jr. or Villains shows. The only figure is 600 for the shows
  these theaters used to host (2017), which is weak because both rooms have since been rebuilt or replaced.
- **Restaurants:** no seat counts.
- **Character meets:** no times per party. Mickey and Minnie meet in two rooms, one after the other; the single-meet model still fits
  the 31-min target.
- **2025 attendance:** the TEA 2025 report is not out; it's probably due in late October 2026.
- **Show run times:** close to what the park file already has (Little Mermaid ~20, Villains ~15 min); the 2026 schedules were kept.

## Park changes
- **Added:**
  - dining: BaseLine Tap House and Ice Cold Hydraulics (Grand Avenue, which is now split at a new junction j110a where they
    stand), Epic Eats, Market (Pixar Place), Sunshine Day Bar, Joffrey's on Sunset;
  - Jessie's Trading Post, as a Toy Story Land shop node;
  - The Magic of Disney Animation: Once Upon a Studio (film), Olaf Draws!, and an Off the Page! character meet.

  Seats, times and appeals are all `[assumed]` and not calibrated: no Queue-Times target exists yet for the new area.
- **Kept:** Oscar's Super Service was already part of the Hollywood Boulevard shops group (map 4).
- **Removed:** Vacation Fun (Jason).
- **Rename:** noted in the file header. Node ids keep `animation_courtyard`, as the 2026 guide map legend still does.

## Alien Swirling Saucers (full model)
- **Archetype:** `simulator_bays`, with each side a bay of 11 two-seat cars. A party takes whole cars; groups of three with small
  children are not modeled.
- **Cycle:** 30 + 50 + 10 + 90 s (only the 3-minute total and the 90 s ride are sourced). That gives 880/hr with every seat full;
  the park's party mix carries ~695/hr.
- **Re-fit (approved):** appeal 0.49 -> 0.56, giving 15.9 / 29.4 min against Queue-Times' 16 / 30.
- **Not on:** `side_down` (one side out) exists at an `[assumed]` rate and stays off in the DHS days, as with the other rides.

## Medium day after M16 (10 seeds, posted avg / busiest hour, min)
| Ride | Queue-Times 2026 | Sim |
|---|---|---|
| Slinky Dog Dash | 63 / 83 | 60.5 / 88.2 |
| Rise of the Resistance | 47 / 65 | 49.2 / 65.9 |
| Rock 'n' Roller Coaster | 45 / 63 | 44.0 / 59.2 |
| Runaway Railway | 34 / 45 | 34.4 / 50.7 |
| Tower of Terror | 30 / 53 | 29.3 / 46.2 |
| Toy Story Mania | 30 / 51 | 28.9 / 43.6 |
| Smugglers Run | 29 / 47 | 29.2 / 42.5 |
| Alien Swirling Saucers | 16 / 30 | 15.9 / 29.4 |
| Star Tours | 7 / 13 | 6.8 / 17.5 |
| Meets: Mickey & Minnie, Olaf, Edna | 31, 15, 14 | 30.0, 16.0, 12.7 |

Every ride is now a full model, and every one with a target lands within 2.5 min of its average.

## Challenges re-set
The turn-away fix raised every baseline, so the M15 goals were re-set with the same rule: the no-lever day misses, and a known play
meets them all.
- **Tame Slinky:** score floor 50.25, at least 8,400 passes used; the Slinky goals are unchanged. No levers meets 2 of 4.
- **Peak day:** score at least 36.4, at most 19,000 balks, at least 10,900 used. No levers meets 1 of 3. The winning play is Tier 1
  inventory 0.2 plus merge 1:1.
- **Daily:** score floor 48.5.

## To observe on the next visit `[user]`
- Walkway widths: the Animation Courtyard (Walt Disney Studios) steps, the Grand Avenue connector, and the Toy Story Land and
  Galaxy's Edge entrances.
- Alien Swirling Saucers: whether both sides run, and the time to load and unload a side.
- Theater fill for Little Mermaid, Disney Jr. and Villains (seats or a full-house headcount).
- Tower of Terror load/unload and drop cycle; Rise group sizes and holding cells; Toy Story Mania tram spacing; stops per hour and
  reset time on any coaster.
