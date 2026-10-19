<!-- Generated: 2026-10-19 | Files scanned: 2 (index.html, server.go) | Token estimate: ~350 -->
# Frontend (web control room, M8)

One page: `internal/web/static/index.html` (357 lines, plain HTML + inline SVG + vanilla JS),
embedded in the binary and served at `/` by `ridesim serve`. No build step, no framework.

## Flow
```
connect() --EventSource /api/events--> render(snapshot)
   render -> renderMap (SVG park graph, node load/status colors)
          -> renderRides (per-ride posted wait + spark sparkline)
          -> renderFeed (live.feed event ticker)
          -> renderSpeeds
controls --post()--> /api/speed | /api/lever | /api/jump ; toast() for results
export link -> GET /api/export
```

## Helpers
clock, el, svg (DOM builders), loadColor, statusColor, shortName

## State
Server-owned: the page holds only the latest snapshot. Node x/y come from the park file.
Mockups: https://claude.ai/artifact/A5DmkWDkphfcvzCJ2y8YeB
