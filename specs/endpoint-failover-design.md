# Endpoint failover by measured latency — design

Status: draft (proposal). Scope: per-peer multiple endpoint candidates,
latency probing, automatic switching.

---

## 1. Current state (verified in v2.2.0 source)

- Peer endpoint is a **single string**: `internal/app/app.go:146`
  `Endpoint string \`json:"endpoint"\``. One endpoint per peer; there is no
  candidate list and no switching anywhere in the tree.
- Latency probe lives in `internal/helper/events.go`:
  `probeCandidatesForTunnel()` builds the candidate list, `runOneLatencyProbe()`
  pings **every** candidate **concurrently** on a 30 s cadence and stores the
  outcome in `h.latencyByTunnel[tunnelName]` /
  `h.latencyProbeByTunnel[tunnelName]`, broadcast in the status event.
- Candidate list (changed 2026-09) — all rows, probed every cycle:
  1. **full tunnel** (a peer claims `0.0.0.0/0` / `::/0`) → `8.8.8.8` and
     `223.5.5.5` (one global + one mainland-China anycast address, since a
     full tunnel is supposed to carry everything). Split tunnels have **no**
     public rows.
  2. the peer `endpoint` — always probed;
  3. a `user-provided value`, when set (de-duplicated against the above).
- The former "/32 host from AllowedIPs" branch is gone: that host is
  frequently offline (a sleeping NAS, a laptop), which made healthy tunnels
  look dead.
- Why every candidate instead of "first one that answers wins": each row
  answers a different question. A silent public probe next to a healthy
  endpoint means "tunnel up, ICMP filtered"; a healthy public probe with a
  dead endpoint means "peer unreachable but traffic still flows". A single
  blended number cannot express either.
- Writing a probe target is resolved then validated: `resolveProbeTarget()`
  resolves a hostname (4 s bound) at save time and
  `validateProbeTarget()` rejects, on a **split tunnel**, any address —
  literal or resolved — outside AllowedIPs, because such a probe never enters
  the tunnel. Full tunnels accept anything. An unresolvable name is rejected
  too (it would only ever produce a permanent "—" row).
- Each cycle records the per-candidate breakdown
  (`latency_probe_results[]`: target, resolved_ip, kind, reachable,
  latency_ms) plus the aggregate `latency_probe_target` /
  `latency_probe_state`. The UI renders one colour-coded row per candidate
  (green ≤ 100 ms, amber ≤ 300 ms, red above or unreachable) and a summary
  line when none of them answer — a real health signal the previous
  single-number display could not carry.

## 2. What "conflicts with the display-only design" means

Three concrete conflicts, not a vague objection:

1. **Role change.** Today the probe is a *passive measurement* whose only
   consumer is the UI. Auto-switching turns the same loop into a *control
   loop* that mutates live tunnel state.
2. **Meaning of the field.** `latency_probe_target` currently means "the
   address to ping for the displayed number". Under switching it would have to
   mean "the candidate set / the threshold to switch on" — a different data
   model. It must be **redefined or a new field added**, not silently
   reinterpreted (otherwise existing users' saved values change meaning under
   them).
3. **Arbitration with the reconnect monitor.** `internal/reconnect/monitor.go`
   already reacts to wake / primary-interface changes by tearing down and
   rebuilding tunnels. A second actor that also mutates peer endpoints can
   fight it: a switch during an in-flight reconnect, or an endpoint change that
   looks like a network change and triggers yet another reconnect. Explicit
   arbitration is required.

## 3. Design

### 3.1 Data model — do NOT change the WireGuard config format

Keep `Endpoint` as the **active** endpoint in the `.conf`, so the file stays
valid for `wg`, `wg-quick` and any other client. Store candidates in tunnel
metadata (`<userdir>/tunnels/<name>.meta.json`, next to `LatencyProbeTarget`):

```json
{ "endpoint_candidates": ["hostA:51820", "hostB:51820"] }
```

First element = preferred. Absent or empty ⇒ behaviour is exactly today's
(no switching, zero risk to existing users).

**Rejected alternative:** comma-separated endpoints inside `Endpoint`.
Non-standard, breaks strict parsers, and destroys interoperability.

### 3.1.1 FAQ — shape of the candidates

**Is every endpoint its own entry?** Yes, at both layers:

- *Storage:* one array element per candidate, each carrying the full
  `host:port` value — `["hostA:51820", "hostB:51820"]`. Never
  `"hostA:51820,hostB:51820"` inside a single string.
- *UI:* one row per candidate (an "add"-able list with a remove button per
  row), so each address is editable, reorderable and individually deletable.
  A single free-text comma-separated box would reintroduce the parsing
  ambiguity we just rejected in the storage layer.

**Does the existing "Latency probe address" box stay?** Yes — it is kept,
unchanged in meaning, and the new failover UI is a **separate** setting:

| Setting | Meaning | Consumed by |
| --- | --- | --- |
| Latency probe address (`latency_probe_target`) | one address pinged to produce the displayed latency number | display only |
| Endpoint candidates (`endpoint_candidates`) | list of `host:port` the tunnel may use | switching logic |

The two must not be merged. Merging would silently change the meaning of a
value existing users already saved, and would tie a display preference to a
control loop. If both are configured, failover probes the candidates and the
probe address remains purely cosmetic.

**Does `Endpoint` in the `.conf` change?** No. It keeps holding the currently
active endpoint; failover rewrites it (in memory + `.conf`) only when it
actually switches.

### 3.2 Probing

Extend the existing 30 s loop instead of adding a second ticker:

- For each candidate: resolve, then `diag.PingEndpoint(host)` (ICMP).
  Endpoint hosts are reachable over the underlay, so ICMP RTT is the correct
  "which peer is closer" metric.
- Keep a small ring buffer per candidate (last ~5 samples) and use the
  **median** so one bad sample cannot cause a switch.
- Candidate fails `consecutive_failures` (default 3) ⇒ mark down.

### 3.3 Switch policy (anti-flap)

Switch from current to best only when one of:

- **Failover** — current is down (N consecutive failures) and another
  candidate is up.
- **Improvement** — current latency > `switch_threshold_ms` (default 150) AND
  the best candidate is better by more than `improvement_margin_pct`
  (default 30 %).

Plus guards:

- **Cooldown** — at least `switch_cooldown` (default 5 min) between switches.
- **Arbitration** — never switch while the reconnect monitor has a reconnect
  in flight (`Pause()`/`Resume()` or a state check on the monitor).

### 3.4 Applying a switch — do NOT tear the tunnel down

Change only the peer endpoint over the UAPI:

```
set=1
<base64 peer public key>
endpoint=<host>:<port>
```

This is what `wg set <iface> peer <key> endpoint …` does: the interface stays
up, only subsequent packets go to the new address and WireGuard re-handshakes.
No adapter rebuild, no reconnect, no route churn.

### 3.5 Logging

Structured events (the project requires detailed, correctly-categorised logs):

| Event | Level | Fields |
| --- | --- | --- |
| `endpoint failover` | Info | `tunnel`, `from`, `to`, `from_ms`, `to_ms`, `reason` (failover/improvement) |
| `endpoint candidate unreachable` | Debug | `tunnel`, `candidate`, `consecutive_failures` |
| `endpoint switch skipped` | Debug | `tunnel`, `reason` (cooldown / reconnect_in_flight / no_better_candidate) |

A switch is rare and user-visible ⇒ Info. Per-sample results ⇒ Debug.

### 3.6 UI

- Tunnel editor: candidate list editor (add / remove `host:port`), an
  "auto switch" toggle, and a measured-latency readout per candidate.
- Tunnel detail: show which candidate is currently active.

### 3.7 i18n

New keys under `tunnel.*` for the candidate list, the toggle and the latency
column — all 5 languages.

## 4. Effort and risk

- **Medium**: meta field + probe-loop extension + switch controller + UAPI
  call + UI + i18n + tests — roughly 400–600 lines across Go and Svelte.
- **Risks and mitigations**
  - Flapping → median + hysteresis + cooldown.
  - ICMP blocked on the path → no switch happens (safe default), logged.
  - Both candidates resolve to the same DDNS host → warn in the UI.
- **Backward compatibility**: with no `endpoint_candidates` configured, the
  behaviour is byte-identical to today.

## 5. Open questions

1. **Sticky or revert?** Should a switch stay on the new endpoint until it
   fails, or return to the preferred candidate once it recovers?
   *Proposal:* sticky, plus an optional "prefer primary" toggle.
2. **ICMP vs handshake reachability.** ICMP can be blocked while WireGuard
   still works, producing false "down" readings.
   *Proposal:* ship ICMP first; add handshake-based probing later.
