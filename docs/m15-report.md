# M15 results: guest score and challenges (2026-10-19)

The first game milestone (Jason): a guest-experience score, challenge files with goals, two new Lightning Lane levers, challenge
play in the control room, and a local leaderboard that reruns every submitted session to verify its score.

## What was built
- **Levers:** `ll_inventory` (share of a ride's hourly capacity sold per return window, for windows not yet started; passes
  already sold stay valid) and `ll_merge` (whole LL guests per standby guest, 0-10; replaces the authored merge phases on
  modeled rides and the 2/3 cap on simple rides). Single Pass rides refuse `ll_inventory`: their passes are sold before the
  day. Runs without the levers are byte-identical to M14.
- **Score** (`internal/score`, weights in `data/scoring/guest-experience.yaml`, all `[assumed]`): points per guest from the
  event log only. Rides earn 10, shows and meets 6; waits past 20 min cost 0.1/min; balks 2 and reneges 5 per guest (at rides,
  shows and meets only); walking 0.05/min, jams 0.2/min; a lapsed pass 5 per guest on it.
- **Challenges** (`data/challenges/`): a park scenario, the scoring, allowed levers (never `crowd_scale`), a fixed or daily
  seed (the date as YYYYMMDD) and goals on score, experiences per guest, balks, reneges, LL lapsed or used, or a ride's
  posted average or peak hour.
- **Play:** `ridesim serve -challenge FILE [-date]` shows the brief, the running score and the goals (posted-wait goals resolve
  at close), offers only the allowed levers (others get 403), and adds LL steppers to each ride card. The export names the
  verify command.
- **Verify and leaderboard:** `ridesim challenge -file FILE -session EXPORT -name NAME` checks that the session is the
  challenge's day with only allowed levers added, reruns it, scores the rerun and adds it to `out/leaderboard.json` (one
  entry per play: challenge, seed and ops). Without `-session` it prints the no-lever baseline.

## No-lever baselines (seed 1 unless noted)
| Day | Score | Rides/guest | Balks | LL used / lapsed |
|---|---|---|---|---|
| Medium | 48.43 | 6.55 | 30,084 | 8,494 / 3,797 |
| High | 33.56 | | 65,257 | 11,690 / 4,457 |
| Daily medium, 2026-10-19 / 20 / 21 | 47.10 / 47.54 / 47.93 | | 30,379 / 28,609 / 30,867 | ~8,000 / ~3,850-4,220 |

On the medium day the penalties per guest are waits 5.9, balks 2.1, walking 2.7, lapsed LL 2.1, reneges 0.2, jams 0.05.

## Finding: the LL levers barely move the whole-day score
Blanket settings on the medium day land between 48.40 and 48.59 (baseline 48.43). Selling no Multi Pass at all gives the best
score, 48.86, and setting `ll_merge` to 0 everywhere drops it to 40.9. The levers change who rides, not how many, and the score
is mostly set by total capacity. Two consequences, both decided with Jason:
- Challenges set goals on ride waits the levers can move, with the score as a floor.
- A goal on passes used (`ll_redeemed`) stops "sell no LL" from winning: the park still has to serve its paying guests.

The levers do move single rides. Slinky's LL inventory at 0.05 takes its posted average from 62.9 to 54.7 min and its busiest
hour from 91.4 to 68.1.

## Challenges
Each one was checked both ways: the no-lever day misses at least one goal, and a known play meets them all.

| Challenge | Goals | No levers | Known winning play |
|---|---|---|---|
| Tame Slinky (medium) | Slinky avg <= 55, peak <= 72; score >= 48.5; LL used >= 8,300 | 62.9 / 91.4, 48.43, 8,494 (1/4) | Slinky inventory 0.1 + merge 1:1 at opening: 54.6 / 70.5, 48.52, 8,345 (4/4) |
| Peak day (high) | score >= 33.65; balks <= 64,000; LL used >= 11,000 | 33.56, 65,257, 11,690 (1/3) | inventory 0.1 on Slinky, Star Tours, Toy Story Mania, Tower of Terror: 33.68, 63,675, 11,314 (3/3) |
| Daily medium | score >= 47; LL used >= 7,500 | met on the three dates tried | (leaderboard challenge) |

The Tame Slinky play was also made in the browser, exported, verified and entered on a leaderboard. The verified rerun matched the
live score exactly (48.52), and entering the same file twice was refused.

## Gaps and leads
- **Balks are high:** about 30,000 guests at rides, shows and meets on the medium day (Little Mermaid "full" 6,852; Slinky 4,104; Rise 4,031). Another ~23,500 at dining do not score. Guests choose from a board up to 5 min old and turn away on arrival. This is an existing park-model behavior (M7/M9) that the score now makes visible; worth a look before tuning the balk weight.
- **Lever response is uneven on the high day:** small inventory changes move the score both ways. A single seed is chaotic. That's fine for a fixed-seed challenge, but don't read small differences as strategy.
- **Daily seeds:** they vary from day to day (47.1-47.9) by more than any lever moves the score, so daily scores are only comparable within a date.
- **Daily dates are not limited:** a daily challenge verifies any date's seed, so a player could replay many dates and submit the
  easiest. Entries rank per date, so the board stays consistent; limit the accepted dates if the board is ever shared.
- **More room to play:** the whole-day score needs levers that add capacity or steer crowds (posted-wait padding, show schedules, staffing). That's the trade-off milestone, where those levers get costs the score sees.
- **Map labels:** they overlap at Hollywood Studios (pre-existing). The train buttons show on rides without a barn, and the server refuses them.

Goals were re-set on 2026-10-19 after the M16 turn-away fix raised every baseline; see docs/m16-report.md.
