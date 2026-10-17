# M8 results (2026-10-17)

Scope (spec §12): the first front end and live controls for the game. Platform decision (Jason): both,
web first. M8 builds the web control room on an API a terminal console can share later. Mockups:
https://claude.ai/artifact/A5DmkWDkphfcvzCJ2y8YeB

## How to play
```
go run ./cmd/ridesim serve -scenario data/scenarios/park-medium-day.yaml   # http://127.0.0.1:8080/
```
Flags: `-addr` (loopback by default), `-seed`, `-data`. Use the **Export as scenario** button to save the
session, put the file under `data/scenarios/`, and then `ridesim run -scenario` reruns it.

## What was built
- **Park ops** (scenario schema):
  - Park scenarios take `ops` with `add_train` / `remove_train`, `single_rider_open` / `single_rider_close`,
    `close_ride` (with `minutes`) and `crowd_scale` (with `value`, 0 to 2).
  - Each op is checked against its node: a coaster with a barn, a single-rider line, a modeled ride.
  - `Park.Apply` runs the same actions scripted or live. Operations closures use the cause `ops_closure`: the
    board shows the ride closed, guests don't choose it, and posted stats skip the closure, just like long
    downtime.
- **Crowd scale by thinning.** The day is drawn at 2x attendance, and each party keeps a fixed random draw; it
  arrives only while that draw is below scale / 2.
  - Changing the scale never shifts any other random draw. Tested: every event before a mid-day change is
    identical.
  - Parties admitted by a raised scale make their Lightning Lane bookings on arrival.
  - Parties thinned out at opening make no advance bookings.
- **`engine.RunBefore(t)`** runs everything strictly before t. A lever applied next lands ahead of every event
  already scheduled at t. Scripted reruns use the same rule, so live play and reruns interleave identically.
- **`internal/live` sessions:**
  - advance in chunks; apply a lever at the next whole second;
  - jump to any time by replaying from opening with the scenario's ops plus the live ops before that time
    (later live ops are dropped);
  - export the session as a park scenario;
  - snapshot from a read-only park view (tested to leave the run unchanged).

  **A played session's exported scenario reruns byte for byte** after the `run_start` header, which names the
  scenario. This is tested with uneven chunks, three levers and a jump back.
- **Park map positions:** optional `x`/`y` on every park node (all or none, checked). The synthetic park uses the
  mockup layout (approved, `[assumed]`, logged).
- **`internal/web` + `ridesim serve`.** Standard library only, so no new dependency.
  - A ticker every 250 ms advances the sim by speed x elapsed time; speeds are pause, 1x, 10x, 60x, 600x and
    1 h/s. Wall-clock time is used only for pacing in the server.
  - Snapshots stream over server-sent events; speed, levers and jumps are JSON POSTs.
  - The page is embedded in the binary:
    - park map with walkway load colour, spillback halos and ride status rings;
    - ride cards with posted wait ("—" when closed), queue, trains, single-rider and close buttons, and the
      day's posted-wait sparkline;
    - event feed (accessibility holds left out as noise);
    - day scrubber to jump, crowd slider, list of levers this session, export button.
  - **Security:**
    - binds to `127.0.0.1` by default;
    - levers accept only `application/json` (a cross-site page can't send that without a CORS preflight that
      is never granted);
    - bodies are capped at 4 KB with strict JSON decoding;
    - CSP `default-src 'self'` (plus Google Fonts), `frame-ancestors 'none'`, `nosniff`;
    - the page builds every element with `textContent` and DOM calls, never `innerHTML`.
- **Checked live in a browser** (Chrome, 1400 px): closing Everest shows CLOSED with the lever recorded in the
  feed, jump back drops later levers, the exported file validates and runs, no console errors.

## Calibration after the crowd-thinning change (medium, 10 seeds)
Drawing the day at 2x and thinning changes each seed's draws but not the distribution. Results stay within seed
noise of M7:
- Everest 22.3 / 42.0 min posted average / peak hour (M7: 23.4 / 46.1);
- HM 28.5 / 48.2 (M7: 29.2 / 56.0);
- LL share 23 / 29 / 26% (low 13 / 17 / 14%, high 30 / 33 / 37%).

## Decisions (CLAUDE.md, M8)
- Front end: both, web first, on one event-stream API.
- Node coordinates live in the park file. Played sessions export as park scenarios with ops. Jumping back
  replays from opening and drops later live levers.
- Crowd scale thins a day drawn at 2x attendance.

## Known gaps / next
- **Terminal console:** later, on the same `/api/state`, `/api/events` and lever API.
- **The map shows edge load, not individual guests.** Animating parties along paths would need positions in the
  snapshot (about 15k parties); a sampled or aggregated version is the likely next step.
- **One session per server.** Several people watching the same server see and control the same day.
- **Jumping back replays from opening.** A full day takes about 1.5 s, so a late jump pauses briefly.
  Checkpoints could remove that if it matters.
- **Levers act on modeled rides only.** `ride_simple` nodes, dining and the theater have no levers yet.
- **Export doesn't preserve study scaling or ad hoc flags,** only scenario fields and ops.
