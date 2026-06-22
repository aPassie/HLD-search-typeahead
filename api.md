# API Documentation

Base URL (local): `http://localhost:8080`. All responses are JSON (`Content-Type: application/json`); the static UI is served from `/`. Query strings are normalized (trimmed + lowercased) server-side, so `IPhone`, ` iphone `, and `iphone` are equivalent.

---

## `GET /suggest`

Prefix suggestions, up to `TOP_K` (default 10).

| Param | Required | Default | Notes |
|---|---|---|---|
| `q` | no | — | the typed prefix; empty/missing → returns trending |
| `mode` | no | server default (`recency`) | `count` (all-time popularity) or `recency` (popularity + recent activity) |

**Request**
```
GET /suggest?q=ip&mode=count
```
**200 OK**
```json
[
  {"query":"iphone","score":100000},
  {"query":"iphone 15","score":85000},
  {"query":"ipad","score":70000}
]
```
- `score` is the value the chosen `mode` ranked by: the raw count in `count` mode, or the combined `log(1+count) + α·recency` in `recency` mode.
- No matches → `200 []`. Empty `q` → the trending list (same shape).

**Recency example** — after `javascript` was searched heavily:
```
GET /suggest?q=jav&mode=recency
```
```json
[
  {"query":"javascript","score":111.41},
  {"query":"java rocks","score":67.50},
  {"query":"java","score":11.46}
]
```

---

## `POST /search`

Record a submitted search and return the dummy response. The query is buffered and written in a batch, so its effect on `/suggest` and `/trending` appears after the next flush (≤ `FLUSH_INTERVAL`).

**Request**
```
POST /search
Content-Type: application/json

{"q":"iphone"}
```
**200 OK**
```json
{"message":"Searched"}
```
**400 Bad Request** (empty/blank query)
```json
{"error":"empty query"}
```

---

## `GET /trending`

Top queries by recent activity (time-decayed), up to `TOP_K`.

**Request**
```
GET /trending
```
**200 OK**
```json
[
  {"query":"javascript","score":50.0},
  {"query":"java rocks","score":32.0}
]
```
- `score` is the effective (decayed-to-now) recency value. Empty until searches have been recorded.

---

## `GET /cache/debug`

Shows how a prefix routes on the consistent-hash ring and whether it is currently cached. **Non-mutating** and **excluded** from hit/miss metrics.

| Param | Required | Notes |
|---|---|---|
| `prefix` | yes | the prefix to inspect (empty → `400`) |

**Request**
```
GET /cache/debug?prefix=ip
```
**200 OK**
```json
{
  "prefix": "ip",
  "key": "ip",
  "hash": 10771293625506255592,
  "node": "cache-1",
  "vnode_pos": 10772485985601381633,
  "state": "hit"
}
```
- `key` is the normalized prefix; `hash` is its 64-bit ring position; `node` is the owning logical cache node; `state` ∈ `hit | miss | expired`.
- A `/suggest` for a cold prefix flips a subsequent probe from `miss` to `hit`.

---

## `GET /metrics`

Operational counters for the performance report.

**Request**
```
GET /metrics
```
**200 OK**
```json
{
  "latency": {
    "cached": {"count":367847,"avg_us":11,"p50_us":25,"p95_us":25,"p99_us":50},
    "cold":   {"count":477,"avg_us":41,"p50_us":25,"p95_us":100,"p99_us":400}
  },
  "cache":  {"hits":367847,"misses":477,"hit_rate":0.9987,"nodes":{"cache-0":179,"cache-1":141,"cache-2":153}},
  "writes": {"received":6000,"dropped":0,"upserts":144,"flushes":12,"reduction_factor":41.67},
  "db":     {"reads":150000,"writes":144}
}
```
- `latency` — server-side `/suggest` time in microseconds, split by cache hit (`cached`) vs miss (`cold`).
- `cache` — hit/miss counters, hit rate, and per-node entry counts.
- `writes` — batch-writer counters; `reduction_factor` = searches per DB write.
- `db` — rows read (startup Trie build) and written (runtime UPSERTs).

---

## Static UI

`GET /` (and `/app.js`, `/style.css`) serve the embedded single-page UI (`//go:embed`). No build step or separate server.

## Status codes

| Code | When |
|---|---|
| `200` | success (including empty result arrays) |
| `400` | `POST /search` or `GET /cache/debug` with an empty query/prefix |
| `404` | unknown path with no static asset |
