# Herald — Product Requirements Document

**Version:** 1.1.0  
**Status:** Active  
**Author:** Akshat Chauhan  
**Started:** July 2026  
**Target Completion:** 30 days  
**Last Updated:** Stack changed from Java Spring Boot + Rust JNI → Go (Chi) + Go propagation core

---

## Table of Contents

1. [Overview](#overview)
2. [Problem Statement](#problem-statement)
3. [Goals & Non-Goals](#goals--non-goals)
4. [Architecture Summary](#architecture-summary)
5. [Functional Requirements](#functional-requirements)
6. [Non-Functional Requirements](#non-functional-requirements)
7. [Tech Stack](#tech-stack)
8. [Feature Roadmap](#feature-roadmap)
9. [Day-by-Day Build Plan](#day-by-day-build-plan)
10. [Database Schema](#database-schema)
11. [API Surface](#api-surface)
12. [CLI Command Reference](#cli-command-reference)
13. [Observability Requirements](#observability-requirements)
14. [Testing Strategy](#testing-strategy)
15. [Definition of Done](#definition-of-done)

---

## Overview

Herald is a **self-hosted, distributed config propagation engine** that pushes typed configuration changes to thousands of running microservices in under 100ms — with consensus-backed delivery, per-service targeting, vector clocks, quorum acks, and a one-command rollback state machine.

It is not a deployment tool. It operates at runtime, alongside live services, without requiring restarts or redeployments.

---

## Problem Statement

Engineering teams running microservices face a recurring problem: changing a configuration value in production requires either a full redeployment (5–10 minutes, downtime risk) or manual server edits (untracked, no rollback). Tools like etcd and Consul store key-value pairs but offer no delivery guarantees, no per-service targeting, no schema validation, and no rollback primitives. SaaS solutions like LaunchDarkly cost hundreds per month and only cover boolean feature flags.

**Herald solves this by treating config propagation as a distributed systems problem** — with consensus, delivery acknowledgment, causal ordering, and fault tolerance built into the core.

---

## Goals & Non-Goals

### Goals

- Push config changes to running services in **< 100ms p99**
- **Consensus-backed** — no change is committed without quorum ack
- **Per-service targeting** via label selectors (`env=prod`, `region=us-east`)
- **Schema validation** at admission time — bad types are rejected before propagation
- **Vector clocks** — no service applies a change out of causal order
- **Rollback in one command** — re-propagates a previous version through consensus
- **Audit log** — every push is permanently recorded with who, what, when, and which services
- **Fault tolerant** — cluster remains available with N/2 node failures
- **Self-hosted** — runs entirely on your infrastructure, no external SaaS dependency
- **Developer-friendly** — `docker compose up` to get started, typed SDK for Node.js

### Non-Goals

- Not a secrets manager (no encryption at rest in v1)
- Not a deployment orchestrator (does not restart services)
- Not a feature flag SaaS (no UI dashboard in v1, CLI only)
- Not a service mesh (does not handle traffic routing)
- Not a replacement for Kubernetes ConfigMaps for static boot config

---

## Architecture Summary

```
┌─────────────────────────────────────────────────┐
│                  Client Tier                    │
│   Go CLI (herald) · TypeScript SDK · gRPC/REST  │
└──────────────────────┬──────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────┐
│         Control Plane  (Go + Chi)               │
│                                                 │
│  ┌─────────────┐  ┌──────────────┐  ┌─────────┐ │
│  │    Raft     │  │ Propagation  │  │Targeting│ │
│  │   Module    │  │    Core      │  │ Engine  │ │
│  │ (goroutines)│  │(goroutines + │  │         │ │
│  │             │  │  channels)   │  │         │ │
│  └─────────────┘  └──────────────┘  └─────────┘ │
│                                                 │
│  ┌─────────────┐  ┌──────────────┐  ┌─────────┐ │
│  │  Heartbeat  │  │Ack Aggregator│  │Rollback │ │
│  │   Monitor   │  │              │  │   FSM   │ │
│  └─────────────┘  └──────────────┘  └─────────┘ │
└──────────────────────┬──────────────────────────┘
                       │ SSE fan-out
        ┌──────────────┼──────────────┐
        ▼              ▼              ▼
  ┌──────────┐   ┌──────────┐   ┌──────────┐
  │ Go Agent │   │ Go Agent │   │ Go Agent │
  │ (sidecar)│   │ (sidecar)│   │ (sidecar)│
  └────┬─────┘   └────┬─────┘   └────┬─────┘
       └───────────────┴──────────────┘
                       │ acks
┌──────────────────────▼──────────────────────────┐
│              PostgreSQL 16                      │
│  config_versions · raft_log · ack_states        │
│  targeting_rules · audit_log · agent_registry   │
│  delivery_failures · schema_registry            │
└─────────────────────────────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────┐
│          Observability                          │
│      Prometheus scrape · Grafana dashboards     │
└─────────────────────────────────────────────────┘
```

### Module layout (Go monorepo)

```
herald/
├── cmd/
│   ├── control-plane/     # main.go — control plane binary
│   ├── agent/             # main.go — subscriber agent binary
│   └── cli/               # main.go — herald CLI binary
├── internal/
│   ├── raft/              # leader election, log replication, RequestVote
│   ├── propagation/       # fan-out engine, delta compression, ordering
│   ├── targeting/         # label selector parser + evaluator
│   ├── fsm/               # rollback finite state machine
│   ├── aggregator/        # ack aggregator, quorum tracking, SLA timer
│   ├── heartbeat/         # heartbeat monitor, stale detection
│   ├── schema/            # schema registry, type + constraint validation
│   ├── vectorclock/       # vector clock data structure + causal comparison
│   ├── audit/             # audit log writer
│   ├── store/             # PostgreSQL repository layer (sqlx)
│   └── api/               # Chi router, REST handlers, gRPC server
├── sdk/
│   └── typescript/        # herald-sdk npm package
├── migrations/            # SQL migrations (golang-migrate)
├── benchmarks/            # k6 scripts
├── deploy/
│   └── docker-compose.yml
└── proto/
    └── herald.proto
```

---

## Functional Requirements

### FR-01: Cluster Bootstrap

- The system SHALL support initializing a cluster of 3 or 5 control plane nodes
- Nodes SHALL elect a leader via Raft within 300ms of startup
- The leader SHALL be the sole writer; followers SHALL redirect writes to the leader
- Cluster state SHALL be persisted in PostgreSQL (`raft_log` table)
- Each node SHALL have a unique ID supplied via environment variable `HERALD_NODE_ID`

### FR-02: Config Write

- A client SHALL be able to push a typed key-value config entry
- Every write SHALL include: key, value, namespace, optional TTL, optional targeting labels
- The control plane SHALL validate the value against the key's registered schema before accepting
- The write SHALL be rejected if schema validation fails, with a human-readable error message
- The write SHALL be routed to the Raft leader; followers SHALL respond with a redirect, not proxy silently

### FR-03: Consensus & Replication

- Every committed config version SHALL be replicated to a majority of control plane nodes via Raft log append
- A write SHALL NOT be acknowledged to the client until quorum is reached on the Raft log
- Each Raft log entry SHALL contain: term, index, config_version_id, timestamp
- Node failure SHALL NOT cause any committed entry to be lost

### FR-04: Propagation

- Upon commit, the control plane SHALL fan out the new config version to all matching subscriber agents
- Fan-out SHALL be performed concurrently using goroutines with bounded worker pools
- Propagation SHALL send only the delta (changed keys) between the previous version and the new version
- Propagation latency from commit to agent ack SHALL be < 100ms p99 under normal conditions

### FR-05: Targeting

- Every config push SHALL support label selector targeting: `env=prod`, `region=us-east`, `service=payments`
- Agents SHALL register their labels on connection
- The targeting engine SHALL evaluate label selectors and return the matching agent set before fan-out
- A push with no targeting labels SHALL propagate to all connected agents in the namespace

### FR-06: Subscriber Agents

- A Go agent (sidecar binary) SHALL connect to the control plane and maintain a persistent SSE connection
- Agents SHALL maintain a local in-memory config cache using `sync.Map` for concurrent read safety
- Agents SHALL apply config changes atomically (all-or-nothing per version) under a `sync.Mutex`
- Agents SHALL send heartbeats every 500ms; the control plane SHALL mark agents STALE after 1500ms silence
- Agents SHALL be idempotent: re-delivery of the same version_id SHALL be a no-op

### FR-07: Acknowledgment & Quorum

- Each agent SHALL send an explicit HTTP ack after applying a config version
- The ack aggregator SHALL track per-agent delivery state in memory + persist to `ack_states`
- A version SHALL be marked COMMITTED when ⌊N/2⌋+1 targeted agents ack
- If quorum ack is not received within 2 seconds, the SLA timer SHALL trigger automatic rollback

### FR-08: Vector Clocks

- Every config version SHALL carry a vector clock (map of node_id → logical timestamp)
- Agents SHALL reject a config version whose vector clock is causally behind the agent's current state
- Out-of-order delivery SHALL be queued per-agent and applied in causal order, not dropped

### FR-09: Rollback

- A client SHALL be able to roll back a key (or full namespace) to any previous committed version
- Rollback SHALL go through the same consensus and propagation pipeline as a forward push
- The rollback FSM SHALL transition through: `ACTIVE → ROLLBACK_PENDING → COMMITTING → ACTIVE`
- Rollback latency SHALL be < 100ms p99

### FR-10: CLI

- A Go CLI binary (`herald`) built with Cobra SHALL expose the following commands:
  - `herald cluster init` — bootstrap cluster
  - `herald cluster status` — show node health, log/commit index, quorum state
  - `herald cluster kill-node <node> --simulate` — fault injection for demo
  - `herald config set <key> <value> --namespace --ttl --target` — push config
  - `herald config get <key> --namespace` — read current value + version + vector clock
  - `herald rollback <key> --to <version> --namespace` — rollback
  - `herald diff <v1> <v2>` — show field-level diff between two versions
  - `herald watch <key>` — live stream value changes via SSE
  - `herald audit --namespace --last N` — show last N audit entries
  - `herald status` — show all agents, their versions, last heartbeat, config hash
  - `herald schema register <key>` — register type + constraints for a key
  - `herald schema list` — list all registered schemas

### FR-11: Schema Registry

- Clients SHALL be able to register a schema for a config key: type (string, int, bool, float, enum), range constraints
- The control plane SHALL enforce schema on every write to that key
- Schema changes SHALL themselves be versioned and audited

### FR-12: Namespaces

- Config keys SHALL be scoped to namespaces (`prod`, `staging`, `dev`)
- A push to one namespace SHALL NOT affect agents in another namespace
- Namespace isolation SHALL be enforced at both the database and propagation layer

### FR-13: Audit Log

- Every config push, rollback, and schema change SHALL be written to an append-only audit log
- Audit entries SHALL contain: timestamp, actor, key, old_value, new_value, version_id, targeting labels, vector clock
- The audit log SHALL never be mutable — a PostgreSQL trigger SHALL prevent UPDATE and DELETE

### FR-14: Fault Injection

- The CLI SHALL support `--simulate` mode for killing nodes and introducing propagation delays
- This is a first-class feature for demos and correctness testing, exposed via a debug endpoint on the control plane

---

## Non-Functional Requirements

### NFR-01: Latency

- Config push to quorum commit: **< 50ms p99**
- Commit to agent ack (propagation): **< 100ms p99**
- Rollback end-to-end: **< 100ms p99**
- CLI command round-trip: **< 200ms**

### NFR-02: Throughput

- Control plane SHALL handle **1,000 config writes/sec** under sustained load
- Propagation core SHALL fan out to **10,000 connected agents** without degradation

### NFR-03: Availability

- Cluster SHALL remain available with up to **N/2 node failures** (Raft majority)
- A single node failure SHALL NOT cause data loss or committed version loss

### NFR-04: Consistency

- Herald guarantees **linearizable writes** — every committed version is seen by all agents in the same causal order
- No agent SHALL ever see a version whose causal predecessor it has not already applied

### NFR-05: Durability

- Every committed config version SHALL be persisted in PostgreSQL before being acknowledged to the client
- The Raft log SHALL be append-only and never mutated after commit

### NFR-06: Observability

- All key operations SHALL emit Prometheus metrics via `prometheus/client_golang`
- A Grafana dashboard SHALL be provisioned automatically via `docker compose up`

### NFR-07: Operability

- `docker compose up` SHALL start a full 3-node cluster, 5 mock agents, PostgreSQL, Prometheus, and Grafana
- The system SHALL have zero external dependencies beyond PostgreSQL

### NFR-08: Testability

- Every distributed systems concept SHALL have a dedicated integration test using `testcontainers-go`
- The benchmark suite SHALL reproduce the numbers claimed in the README

---

## Tech Stack

| Component        | Language / Library          | Responsibility                                                   |
| ---------------- | --------------------------- | ---------------------------------------------------------------- |
| Control plane    | Go 1.22 + Chi               | Raft, REST API, gRPC, targeting, rollback FSM, heartbeat monitor |
| Propagation core | Go (goroutines + channels)  | Concurrent fan-out, delta compression, delivery ordering         |
| Subscriber agent | Go 1.22                     | Sidecar binary, local cache, SSE client, heartbeat, ack          |
| CLI              | Go 1.22 + Cobra + Lip Gloss | All operator commands, colored terminal output                   |
| TypeScript SDK   | TypeScript + Node 20        | Typed client for Node.js services                                |
| Database         | PostgreSQL 16               | All durable state — config, Raft log, acks, audit                |
| Migrations       | golang-migrate              | Schema versioning                                                |
| DB driver        | sqlx + pgx                  | Typed SQL queries, advisory locks                                |
| gRPC             | google.golang.org/grpc      | Control plane ↔ control plane (Raft RPCs)                        |
| Metrics          | prometheus/client_golang    | Instrumentation                                                  |
| Observability    | Prometheus + Grafana        | Dashboards, scraping                                             |
| Container        | Docker + Docker Compose     | Local dev and demo                                               |

---

## Feature Roadmap

### Phase 1 — Foundation (Days 1–5)

- [ ] Go module monorepo: `cmd/`, `internal/`, `sdk/`, `migrations/`, `proto/`
- [ ] PostgreSQL schema: all tables, indexes, constraints, audit trigger
- [ ] Control plane: Chi router skeleton, health endpoint, structured JSON logging (zerolog)
- [ ] Raft leader election v1: PostgreSQL advisory lock acquire/refresh/release
- [ ] `herald cluster init` and `herald cluster status` CLI commands
- [ ] Docker Compose: 3 control plane nodes + PostgreSQL

### Phase 2 — Core Write Path (Days 6–10)

- [ ] Config write endpoint: schema validation, namespace scoping, version creation
- [ ] Raft log append on write: (term, index, config_version_id) replication via goroutines
- [ ] Vector clock generation and assignment on each write
- [ ] `herald config set`, `herald config get`, `herald diff` CLI commands
- [ ] Audit log writer

### Phase 3 — Propagation (Days 11–15)

- [ ] Propagation core: bounded goroutine worker pool, fan-out to N agents concurrently
- [ ] Delta compression: compute and send only changed keys between versions
- [ ] Go subscriber agent: SSE client, `sync.Map` cache, atomic apply under mutex
- [ ] Agent heartbeat: 500ms ticker, 1500ms STALE threshold in control plane
- [ ] `herald watch` CLI command

### Phase 4 — Consensus & Acks (Days 16–20)

- [ ] Agent ack protocol: explicit HTTP ack post-apply, idempotency check
- [ ] Ack aggregator: per-version delivery tracking, quorum threshold
- [ ] SLA timer: 2s timeout → automatic rollback trigger
- [ ] Full Raft log replication: gRPC AppendEntries across 3 control plane nodes
- [ ] Leader re-election on node failure: RequestVote RPC
- [ ] `herald cluster kill-node --simulate` fault injection

### Phase 5 — Rollback & Targeting (Days 21–24)

- [ ] Rollback FSM: `ACTIVE → ROLLBACK_PENDING → COMMITTING → ACTIVE`
- [ ] `herald rollback` CLI command
- [ ] Label selector targeting engine
- [ ] Agent label registration on connect
- [ ] `--target` flag on config set and rollback
- [ ] Schema registry: type + constraint validation on write

### Phase 6 — Observability & Polish (Days 25–28)

- [ ] Prometheus metrics: propagation latency, ack rates, Raft indices, stale agents
- [ ] Grafana dashboard: provisioned via Docker Compose
- [ ] `herald audit` and `herald status` commands
- [ ] TypeScript SDK: `herald.get()` + `herald.subscribe()`
- [ ] README: architecture diagram, demo GIF, benchmark numbers

### Phase 7 — Benchmarks & Hardening (Days 29–30)

- [ ] k6 benchmark suite: throughput, propagation latency, rollback latency, fault tolerance
- [ ] Integration test suite: testcontainers-go, Raft correctness, vector clock ordering, rollback FSM, SLA timer
- [ ] Final README polish

---

## Day-by-Day Build Plan

> Commit counts are variable by complexity. Each commit is named specifically.

### Day 1 — Repo Scaffold `[3 commits]`

- Go module init (`go mod init github.com/AkZcH/herald`)
- Directory layout: `cmd/`, `internal/`, `sdk/`, `migrations/`, `proto/`, `benchmarks/`, `deploy/`
- GitHub Actions CI: `go build ./...` + `go test ./...` on push
- **Commit 1:** go module init + full directory scaffold + .gitignore
- **Commit 2:** Docker Compose skeleton (postgres + 3 control-plane placeholders)
- **Commit 3:** GitHub Actions CI pipeline

### Day 2 — Database Schema `[4 commits]`

- golang-migrate setup, all SQL migrations
- Tables: `config_versions`, `raft_log`, `ack_states`, `delivery_failures`, `targeting_rules`, `audit_log`, `schema_registry`, `agent_registry`
- Append-only trigger on `audit_log`
- **Commit 1:** golang-migrate setup + `config_versions` + `raft_log` migrations
- **Commit 2:** `ack_states` + `delivery_failures` + `agent_registry` migrations
- **Commit 3:** `targeting_rules` + `schema_registry` + `audit_log` migrations
- **Commit 4:** Append-only trigger on `audit_log` + all indexes + seed namespace data

### Day 3 — Control Plane Skeleton `[4 commits]`

- Chi router setup, `/health` endpoint, graceful shutdown
- Structured JSON logging with zerolog (node_id in every line)
- PostgreSQL connection pool with pgx + sqlx
- Node identity from `HERALD_NODE_ID` env var
- **Commit 1:** Chi router + `/health` + graceful shutdown
- **Commit 2:** zerolog structured logging + node identity from env
- **Commit 3:** pgx + sqlx connection pool + ping on startup
- **Commit 4:** Multi-node Docker Compose wiring (3 nodes, env vars, port mapping)

### Day 4 — Raft Leader Election v1 `[5 commits]`

- `internal/raft`: leader election via `pg_try_advisory_lock(42)`
- Background goroutine: attempts lock every 200ms
- Leader heartbeat: refreshes lock every 100ms via `pg_advisory_lock` keepalive
- Follower detection: nodes without lock register as followers
- `/cluster/status` REST endpoint: returns role, term, node_id, uptime
- **Commit 1:** `RaftNode` struct + advisory lock acquire loop
- **Commit 2:** Leader heartbeat goroutine + lock expiry detection
- **Commit 3:** Follower state + write redirect middleware (307 to leader)
- **Commit 4:** `/cluster/status` endpoint
- **Commit 5:** Integration test: 3 nodes start, assert exactly 1 leader

### Day 5 — CLI Foundation `[3 commits]`

- `cmd/cli`: Cobra root command + gRPC/HTTP client to control plane
- Lip Gloss color scheme (green ✓, yellow in-progress, red ⚡, cyan addresses)
- `herald cluster init` and `herald cluster status` commands
- **Commit 1:** Cobra scaffold + HTTP client + color scheme constants
- **Commit 2:** `herald cluster init` command + output rendering
- **Commit 3:** `herald cluster status` command + node health table

### Day 6 — Config Write API `[4 commits]`

- `POST /config`: accepts key, value, namespace, TTL, targeting labels
- Namespace scoping enforced at query level
- Schema validation against `schema_registry` (type, range, enum)
- `config_versions` insert with status=PENDING
- **Commit 1:** Config write handler + request struct + validation
- **Commit 2:** Schema validation service (type + range + enum checks)
- **Commit 3:** `config_versions` insert + namespace scoping
- **Commit 4:** Human-readable rejection errors on schema failure

### Day 7 — Vector Clocks `[4 commits]`

- `internal/vectorclock`: `VectorClock` type (`map[string]uint64`)
- Methods: `Increment(nodeID)`, `Merge(other)`, `HappensBefore(other)`, `Concurrent(other)`
- Clock assigned on every config write, stored as JSONB in `config_versions`
- **Commit 1:** `VectorClock` type + `Increment` + `Merge`
- **Commit 2:** `HappensBefore` + `Concurrent` causal comparison
- **Commit 3:** Clock assignment in config write path
- **Commit 4:** Unit tests: causality, concurrency, merge correctness (table-driven)

### Day 8 — Raft Log Replication `[5 commits]`

- `internal/raft`: leader appends to `raft_log`, broadcasts AppendEntries to followers via gRPC
- `proto/herald.proto`: `AppendEntries` + `AppendEntriesResponse` messages
- Followers write to their local `raft_log`, respond with success/term
- Leader awaits quorum ack before returning to client
- **Commit 1:** `herald.proto` AppendEntries RPC definition + generated Go stubs
- **Commit 2:** Leader `raft_log` append + peer broadcast goroutines
- **Commit 3:** Follower AppendEntries gRPC handler + log write
- **Commit 4:** Quorum ack tracking on leader (channel-based, timeout-gated)
- **Commit 5:** Integration test: write committed only after 2/3 nodes ack log entry

### Day 9 — CLI Config Commands `[3 commits]`

- `herald config set` with `--namespace`, `--ttl`, `--target` flags
- `herald config get` — current value, version, vector clock, status
- `herald diff <v1> <v2>` — field-level colored diff output
- **Commit 1:** `config set` command + response rendering (vector clock display)
- **Commit 2:** `config get` command
- **Commit 3:** `diff` command with green/red field diff lines

### Day 10 — Audit Log `[3 commits]`

- `internal/audit`: writer called on every committed write and rollback
- Appends to `audit_log` with full context (actor, key, old/new value, vector clock, targeting)
- `herald audit --namespace prod --last N` CLI command with table output
- **Commit 1:** Audit writer service + integration into config write path
- **Commit 2:** Verify append-only trigger prevents mutation (test)
- **Commit 3:** `herald audit` CLI command + table rendering

### Day 11 — Propagation Core `[4 commits]`

- `internal/propagation`: `Propagator` struct with bounded goroutine worker pool (`semaphore` pattern)
- Fan-out: for each targeted agent, spawn goroutine from pool, POST delta to agent SSE endpoint
- Context-cancellation: propagation cancels in-flight fan-out on rollback trigger
- **Commit 1:** `Propagator` struct + bounded worker pool (semaphore via buffered channel)
- **Commit 2:** Fan-out loop: goroutine per agent, HTTP POST delta, collect results
- **Commit 3:** Context cancellation on rollback trigger
- **Commit 4:** Unit test: fan-out to 100 mock agents, assert all receive delta

### Day 12 — Delta Compression `[3 commits]`

- Compute diff between `previous_version` and new version: only changed/added/deleted keys
- Delta serialized as JSON (compact, key→{old,new} structure)
- Only delta sent to agents — not full config blob
- **Commit 1:** Delta computation function (`ComputeDelta(prev, next map[string]any)`)
- **Commit 2:** Delta serialization + size comparison logging (delta vs full)
- **Commit 3:** Unit tests: delta correctness across 5 version pair scenarios

### Day 13 — Go Subscriber Agent `[5 commits]`

- `cmd/agent`: connects to control plane SSE endpoint, receives pushed version deltas
- `sync.Map` local config cache, atomic apply under `sync.Mutex`
- Label registration on connect via `POST /agents/register`
- Idempotency: `lastAppliedVersionID` checked before apply
- Agent exposes `GET /config/{key}` for the local service to read without network hop
- **Commit 1:** Agent binary init + SSE client + reconnect loop
- **Commit 2:** `sync.Map` cache + atomic apply under mutex
- **Commit 3:** Label registration on connect
- **Commit 4:** Idempotency check + `lastAppliedVersionID` tracking
- **Commit 5:** Local `GET /config/{key}` endpoint for co-located service reads

### Day 14 — Heartbeat Protocol `[3 commits]`

- Agent: `time.Ticker` at 500ms, `POST /heartbeat` with `{agent_id, version_id, config_hash}`
- Control plane: updates `agent_registry.last_seen` + `config_hash` on receipt
- Control plane background goroutine: scans `agent_registry` every 500ms, marks STALE after 1500ms
- **Commit 1:** Agent heartbeat ticker + POST
- **Commit 2:** Control plane heartbeat handler + `last_seen` update
- **Commit 3:** Stale detection goroutine + STALE state write + exclusion from targeting

### Day 15 — `herald watch` `[2 commits]`

- Control plane: SSE endpoint `GET /config/{key}/watch` — streams `data: {version, value}` on each commit
- `herald watch <key> --namespace` CLI: subscribes, prints each new value live with timestamp
- **Commit 1:** SSE watch endpoint in control plane
- **Commit 2:** `herald watch` CLI command with live rendering

### Day 16 — Agent Ack Protocol `[4 commits]`

- Agent: `POST /ack` with `{agent_id, version_id}` immediately after atomic apply
- Control plane: writes to `ack_states`, updates aggregator state
- Idempotent: second ack for same `(agent_id, version_id)` pair is a no-op
- **Commit 1:** Agent ack sender (called post-apply)
- **Commit 2:** Control plane ack handler + `ack_states` upsert
- **Commit 3:** Ack aggregator: in-memory map `versionID → {agentID → bool}`
- **Commit 4:** Unit test: ack idempotency + aggregator state transitions

### Day 17 — Quorum Ack & Commit `[4 commits]`

- Quorum: `floor(N/2) + 1` where N = number of targeted agents for that version
- Aggregator: when quorum reached, transitions `config_versions.status` to COMMITTED
- Config write path: client response held on a channel until COMMITTED or timeout
- **Commit 1:** Quorum calculation per targeted set
- **Commit 2:** COMMITTED transition + DB write
- **Commit 3:** Client response channel: write handler blocks until COMMITTED signal
- **Commit 4:** Integration test: 5 agents, quorum at 3, verify commit timing and response

### Day 18 — SLA Timer & Auto-Rollback `[4 commits]`

- `internal/aggregator`: per-version `time.AfterFunc(2s, triggerRollback)`
- On fire: if quorum not yet reached, triggers automatic rollback to previous committed version
- `delivery_failures` record created for each non-acking agent
- Timer cancelled immediately on quorum success
- **Commit 1:** Per-version SLA timer with `time.AfterFunc`
- **Commit 2:** Timer fire handler → rollback trigger
- **Commit 3:** `delivery_failures` writer for non-acking agents
- **Commit 4:** Integration test: 3/5 agents offline → auto-rollback fires at 2s

### Day 19 — Full Raft: RequestVote & Re-election `[5 commits]`

- Upgrade Raft from advisory-lock-only to full RequestVote + AppendEntries
- Election timeout: randomized `150ms + rand(0,150ms)` per node to avoid split votes
- Candidate state: increments term, votes for self, sends RequestVote to peers
- Vote granting: follower grants if candidate term ≥ current term and log is up-to-date
- `herald cluster status` shows updated term after re-election
- **Commit 1:** `RequestVote` RPC in `herald.proto` + generated stubs
- **Commit 2:** Candidate state machine + term increment + self-vote
- **Commit 3:** Vote granting logic (term comparison + log completeness check)
- **Commit 4:** Election timeout goroutine with randomization
- **Commit 5:** Integration test: kill leader goroutine, assert re-election completes < 500ms

### Day 20 — Fault Injection `[3 commits]`

- `POST /debug/partition/{node_id}`: marks node as unreachable in cluster state
- Control plane responds with UNREACHABLE node in cluster status
- Log catch-up on reconnect: leader replays AppendEntries from follower's `log_index`
- **Commit 1:** Debug partition endpoint + cluster state update
- **Commit 2:** Unreachable node exclusion from Raft quorum + status display
- **Commit 3:** Log catch-up on reconnect (replay from `commit_index` gap)

### Day 21 — Rollback FSM `[4 commits]`

- `internal/fsm`: `RollbackFSM` with states `ACTIVE | ROLLBACK_PENDING | COMMITTING | ROLLED_BACK`
- Transition table enforced: invalid transitions return error
- FSM state stored in `config_versions.rollback_status`
- Guard: concurrent rollbacks on same key are rejected
- **Commit 1:** `RollbackFSM` type + transition table + `Transition(event)` method
- **Commit 2:** FSM integration into rollback handler
- **Commit 3:** Concurrent rollback guard (mutex per key)
- **Commit 4:** Unit tests: all valid transitions, all invalid transition rejections

### Day 22 — `herald rollback` CLI `[3 commits]`

- `herald rollback <key> --to <version> --namespace`
- Output: FSM state transitions printed live (`ACTIVE → ROLLBACK_PENDING → COMMITTING → ACTIVE`)
- Error cases: version not found, not committed, concurrent rollback in progress
- **Commit 1:** Rollback CLI command + HTTP call to control plane
- **Commit 2:** Live FSM transition rendering (poll `/rollback/status/{id}`)
- **Commit 3:** Error case handling + user-friendly output

### Day 23 — Targeting Engine `[4 commits]`

- `internal/targeting`: label selector parser (`env=prod,region=us-east` → predicate list)
- Agent label index: in-memory `map[agentID]Labels`, updated on heartbeat
- Evaluator: returns matching agent set for a given selector string
- `--target` flag wired into config set, rollback, watch
- **Commit 1:** Label selector parser + predicate type
- **Commit 2:** In-memory agent label index + update on heartbeat
- **Commit 3:** Targeting evaluator: selector → `[]AgentID`
- **Commit 4:** Integration test: 10 agents with mixed labels, assert targeting precision

### Day 24 — Schema Registry `[3 commits]`

- `internal/schema`: `Validate(key, namespace, value)` checks type, range, enum
- Schema registration: `POST /schema` persists to `schema_registry`
- `herald schema register` and `herald schema list` CLI commands
- **Commit 1:** Schema registry store + registration endpoint
- **Commit 2:** Schema enforcement in config write path
- **Commit 3:** `herald schema` CLI commands

### Day 25 — Prometheus Metrics `[4 commits]`

- `prometheus/client_golang` registered on Chi router at `/metrics`
- Metrics: propagation latency histogram, ack rate gauge, stale agent gauge, Raft term gauge, Raft commit index gauge, delivery failure counter, rollback counter, quorum miss counter
- Agent metrics: apply latency histogram, cache size gauge, heartbeat counter
- **Commit 1:** Prometheus setup + propagation latency histogram
- **Commit 2:** Raft metrics (term, commit index, leader uptime)
- **Commit 3:** Agent metrics (apply latency, cache size)
- **Commit 4:** Delivery failure counter + stale agent gauge + rollback counter

### Day 26 — Grafana Dashboard `[3 commits]`

- Grafana datasource + dashboard JSON in `deploy/grafana/`
- Panels: propagation latency p50/p99 time series, ack heatmap per agent, Raft term timeline, stale agent count, delivery failure rate, rollback rate
- Docker Compose auto-provisions Grafana on startup
- **Commit 1:** Grafana datasource provisioning config
- **Commit 2:** Dashboard JSON — propagation latency + ack heatmap panels
- **Commit 3:** Dashboard JSON — Raft health + stale agents + failure panels

### Day 27 — `herald status` Polish `[3 commits]`

- `herald status`: table of all agents — agent_id, version, last_seen, config_hash, labels, status
- Config hash: SHA-256 of the full applied config blob, computed by agent, sent in heartbeat
- `herald audit` pagination (`--page`, `--last N`) + namespace filter
- **Commit 1:** Config hash in agent heartbeat + `/agents` endpoint
- **Commit 2:** `herald status` table rendering with config hash column
- **Commit 3:** `herald audit` pagination + filter flags

### Day 28 — TypeScript SDK `[4 commits]`

- `sdk/typescript`: npm package `herald-sdk`
- `herald.get(key, namespace)` — HTTP GET to agent's local endpoint, returns typed value
- `herald.subscribe(key, namespace, callback)` — SSE subscription, fires callback on change
- Reconnection with exponential backoff on SSE drop
- **Commit 1:** SDK init + HTTP client + `herald.get()`
- **Commit 2:** `herald.subscribe()` SSE client + callback dispatch
- **Commit 3:** Reconnection logic + exponential backoff
- **Commit 4:** TypeScript types generated from JSON schema of `config_versions`

### Day 29 — Benchmark Suite `[4 commits]`

- `benchmarks/`: k6 scripts for 4 scenarios
- Benchmark 1 (throughput): sustained 1000 writes/sec for 60s, measure p50/p99
- Benchmark 2 (propagation): 5 / 50 / 500 agents, commit-to-all-ack latency
- Benchmark 3 (rollback): end-to-end rollback latency under concurrent write load
- Benchmark 4 (fault tolerance): kill leader mid-run, measure re-election time + write continuity
- **Commit 1:** k6 setup + throughput benchmark script
- **Commit 2:** Propagation latency benchmark (3 agent-count variants)
- **Commit 3:** Rollback latency benchmark
- **Commit 4:** Fault tolerance benchmark + results table committed to `benchmarks/RESULTS.md`

### Day 30 — Integration Tests & Final Polish `[5 commits]`

- `testcontainers-go`: spin up real PostgreSQL in tests, no mocks for DB layer
- Test: Raft correctness — commit only after quorum, no commit on single node
- Test: Vector clock ordering — out-of-order delivery queued, applied in causal order
- Test: Rollback FSM — all state transitions, invalid transition rejection
- Test: SLA timer — auto-rollback fires at exactly 2s on quorum miss
- Test: Stale agent — missed heartbeat → STALE → excluded from next push targeting
- README: architecture diagram, demo GIF, benchmark table, install guide, `docker compose up` quickstart
- **Commit 1:** Raft correctness integration test (testcontainers-go)
- **Commit 2:** Vector clock ordering integration test
- **Commit 3:** Rollback FSM + SLA auto-rollback integration test
- **Commit 4:** Stale agent + targeting integration test
- **Commit 5:** README final pass + benchmark table + demo GIF

---

## Database Schema

```sql
-- Config versions: append-only after commit
CREATE TABLE config_versions (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key              TEXT NOT NULL,
    namespace        TEXT NOT NULL DEFAULT 'default',
    value            JSONB NOT NULL,
    previous_id      UUID REFERENCES config_versions(id),
    vector_clock     JSONB NOT NULL DEFAULT '{}',
    status           TEXT NOT NULL CHECK (status IN (
                       'PENDING','PROPAGATING','COMMITTED','ROLLING_BACK','ROLLED_BACK'
                     )),
    rollback_status  TEXT,
    ttl_expires_at   TIMESTAMPTZ,
    targeting_labels JSONB,
    pushed_by        TEXT NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_config_versions_key_ns ON config_versions(key, namespace);
CREATE INDEX idx_config_versions_status ON config_versions(status);

-- Raft log: strictly append-only
CREATE TABLE raft_log (
    id                BIGSERIAL PRIMARY KEY,
    term              BIGINT NOT NULL,
    log_index         BIGINT NOT NULL UNIQUE,
    node_id           TEXT NOT NULL,
    config_version_id UUID REFERENCES config_versions(id),
    command           TEXT NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Per-agent ack state per version
CREATE TABLE ack_states (
    version_id UUID REFERENCES config_versions(id),
    agent_id   TEXT NOT NULL,
    acked_at   TIMESTAMPTZ,
    status     TEXT NOT NULL CHECK (status IN ('PENDING','ACKED','FAILED')),
    PRIMARY KEY (version_id, agent_id)
);

-- Failed deliveries
CREATE TABLE delivery_failures (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    version_id UUID REFERENCES config_versions(id),
    agent_id   TEXT NOT NULL,
    reason     TEXT,
    failed_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Agent registry
CREATE TABLE agent_registry (
    agent_id      TEXT PRIMARY KEY,
    namespace     TEXT NOT NULL,
    labels        JSONB NOT NULL DEFAULT '{}',
    status        TEXT NOT NULL CHECK (status IN ('HEALTHY','STALE','DEAD')),
    last_seen     TIMESTAMPTZ NOT NULL,
    version_id    UUID REFERENCES config_versions(id),
    config_hash   TEXT,
    registered_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Audit log: append-only enforced by trigger
CREATE TABLE audit_log (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type   TEXT NOT NULL,
    key          TEXT NOT NULL,
    namespace    TEXT NOT NULL,
    old_value    JSONB,
    new_value    JSONB,
    version_id   UUID REFERENCES config_versions(id),
    vector_clock JSONB,
    actor        TEXT NOT NULL,
    targeting    JSONB,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Trigger: prevent UPDATE or DELETE on audit_log
CREATE OR REPLACE FUNCTION audit_log_immutable()
RETURNS TRIGGER AS $$
BEGIN
  RAISE EXCEPTION 'audit_log is append-only';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER enforce_audit_immutability
BEFORE UPDATE OR DELETE ON audit_log
FOR EACH ROW EXECUTE FUNCTION audit_log_immutable();

-- Schema registry
CREATE TABLE schema_registry (
    key         TEXT NOT NULL,
    namespace   TEXT NOT NULL,
    type        TEXT NOT NULL CHECK (type IN ('string','integer','float','boolean','enum')),
    constraints JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (key, namespace)
);

-- Targeting rules (saved selectors)
CREATE TABLE targeting_rules (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    namespace  TEXT NOT NULL,
    name       TEXT NOT NULL,
    selector   JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

---

## API Surface

### REST (Chi router, control plane)

| Method | Path                         | Description                               |
| ------ | ---------------------------- | ----------------------------------------- |
| GET    | `/health`                    | Liveness probe                            |
| POST   | `/config`                    | Push a new config version                 |
| GET    | `/config/{key}`              | Get current committed value               |
| GET    | `/config/{key}/versions`     | List all versions for a key               |
| GET    | `/config/{key}/watch`        | SSE stream — live value changes           |
| POST   | `/config/{key}/rollback`     | Rollback to a specific version            |
| GET    | `/rollback/status/{id}`      | Poll rollback FSM state                   |
| GET    | `/cluster/status`            | Node roles, Raft indices, quorum state    |
| POST   | `/cluster/init`              | Bootstrap cluster                         |
| POST   | `/debug/partition/{node_id}` | Simulate node partition (debug mode)      |
| GET    | `/audit`                     | Query audit log (`?namespace=&last=`)     |
| POST   | `/schema`                    | Register schema for a key                 |
| GET    | `/schema/{key}`              | Get schema for a key                      |
| GET    | `/agents`                    | List all agents with status + config hash |
| POST   | `/agents/register`           | Agent label registration on connect       |
| POST   | `/heartbeat`                 | Agent heartbeat                           |
| POST   | `/ack`                       | Agent delivery ack                        |
| GET    | `/metrics`                   | Prometheus scrape endpoint                |

### gRPC (herald.proto — control plane ↔ control plane)

```protobuf
syntax = "proto3";
package herald;

service HeraldRaft {
  rpc AppendEntries (AppendEntriesRequest)  returns (AppendEntriesResponse);
  rpc RequestVote   (RequestVoteRequest)    returns (RequestVoteResponse);
}

message AppendEntriesRequest {
  int64  term           = 1;
  string leader_id      = 2;
  int64  prev_log_index = 3;
  int64  prev_log_term  = 4;
  repeated LogEntry entries = 5;
  int64  leader_commit  = 6;
}

message AppendEntriesResponse {
  int64 term    = 1;
  bool  success = 2;
}

message RequestVoteRequest {
  int64  term           = 1;
  string candidate_id   = 2;
  int64  last_log_index = 3;
  int64  last_log_term  = 4;
}

message RequestVoteResponse {
  int64 term         = 1;
  bool  vote_granted = 2;
}

message LogEntry {
  int64  term              = 1;
  int64  index             = 2;
  string config_version_id = 3;
  string command           = 4;
}
```

---

## CLI Command Reference

```
herald cluster init     --nodes N --quorum-size N --db <postgres-url>
herald cluster status
herald cluster kill-node <node-id> --simulate

herald config set   <key> <value>  --namespace <ns>  --ttl <duration>  --target <selector>
herald config get   <key>          --namespace <ns>
herald rollback     <key>          --to <version-id>  --namespace <ns>
herald diff         <v1> <v2>
herald watch        <key>          --namespace <ns>
herald audit                       --namespace <ns>   --last N
herald status

herald schema register <key>  --type <type>  --min N  --max N  --enum a,b,c  --namespace <ns>
herald schema list             --namespace <ns>
```

---

## Observability Requirements

### Prometheus Metrics

| Metric                           | Type      | Description                                            |
| -------------------------------- | --------- | ------------------------------------------------------ |
| `herald_propagation_latency_ms`  | Histogram | Commit → all-ack latency (buckets: 10,25,50,100,250ms) |
| `herald_write_latency_ms`        | Histogram | Client push → commit latency                           |
| `herald_ack_rate`                | Gauge     | Acks per second                                        |
| `herald_stale_agents`            | Gauge     | Current agents in STALE state                          |
| `herald_raft_term`               | Gauge     | Current Raft term                                      |
| `herald_raft_commit_index`       | Gauge     | Current commit index                                   |
| `herald_delivery_failures_total` | Counter   | Total delivery failures (cumulative)                   |
| `herald_rollback_total`          | Counter   | Total rollbacks (manual + auto)                        |
| `herald_quorum_misses_total`     | Counter   | Quorum not reached within SLA window                   |
| `herald_agent_apply_latency_ms`  | Histogram | Agent-side apply latency per version                   |
| `herald_agent_cache_size`        | Gauge     | Number of keys in agent local cache                    |

---

## Testing Strategy

### Unit Tests (`go test ./internal/...`)

- `vectorclock`: `HappensBefore`, `Concurrent`, `Merge` — table-driven
- `fsm`: all valid transitions, all invalid transition rejections
- `targeting`: label selector parsing, evaluation against 20 label combinations
- `schema`: type checks, range enforcement, enum validation, rejection messages
- `propagation`: delta computation across 5 version pair scenarios
- `aggregator`: quorum calculation for N=1,3,5,7; ack idempotency

### Integration Tests (`testcontainers-go`)

- Raft leader election: 3 nodes, exactly 1 leader, correct term
- Raft quorum commit: write held until 2/3 nodes append log
- Vector clock ordering: agent receives v3 before v2, queues v3, applies v2 then v3
- Rollback FSM: full state machine traversal, concurrent rollback rejection
- SLA timer: 3/5 agents offline → auto-rollback at 2s confirmed
- Stale agent: heartbeat stops → STALE at 1500ms → excluded from next push
- Full end-to-end: push → propagate → ack → commit → `herald status` shows correct hash

### Benchmarks (k6)

- Throughput: 1000 concurrent writes/sec for 60s — p50, p99 latency
- Propagation: 5 / 50 / 500 agents — commit-to-all-ack latency
- Rollback: under 500 writes/sec concurrent load — rollback p99
- Fault tolerance: kill leader mid-benchmark — re-election time, zero committed write loss

---

## Definition of Done

### Per feature

1. Merged to `main`
2. Unit tests pass, coverage ≥ 80% for that `internal/` package
3. Integration test covers happy path + at least one failure path
4. Relevant CLI command produces correct colored output
5. Relevant Prometheus metric is emitted and visible in Grafana
6. Relevant audit log entry is written and queryable via `herald audit`

### Project complete

1. All Phase 1–7 features checked off in this PRD
2. `docker compose up` starts full cluster in < 60 seconds on a fresh machine
3. Demo script (from the CLI screenshots) runs without errors start to finish
4. Benchmark numbers are in `benchmarks/RESULTS.md` with methodology
5. README has architecture diagram, demo GIF, quickstart, and benchmark table
