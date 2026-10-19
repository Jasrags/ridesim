<!-- Generated: 2026-10-19 | Files scanned: go.mod + imports | Token estimate: ~200 -->
# Dependencies

## Go modules (go.mod, Go 1.22)
- gopkg.in/yaml.v3 v3.0.1 : YAML decode (strict) for config; Node API reserved for comment-preserving rewrites
- Everything else is the standard library (net/http, embed, encoding/json, math/rand). New deps need
  written justification and Jason's approval.

## External data (offline, committed)
- OpenStreetMap Overpass extract (osm/wdw-dhs), ODbL, "© OpenStreetMap contributors"
- Source tags only, no runtime fetches: wiki, rcdb, fan sites, Queue-Times 2026 averages, TEA attendance

## Runtime services
None. `serve` binds 127.0.0.1:8080 locally; no network calls, no database, no auth.
