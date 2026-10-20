<h1 align="center">Herald</h1>

<p align="center">
  <b>Self-hosted configuration propagation for running services.</b><br>
  Write a change once. Every matching agent receives it over a live stream,<br>
  with quorum-committed writes, per-key vector clocks, an append-only audit log and one-command rollback.
</p>

<p align="center">
  <a href="https://github.com/AkZcH/herald/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/AkZcH/herald/actions/workflows/ci.yml/badge.svg"></a>
  <a href="go.mod"><img alt="Go version" src="https://img.shields.io/github/go-mod/go-version/AkZcH/herald"></a>
  <img alt="PostgreSQL 16" src="https://img.shields.io/badge/PostgreSQL-16-336791?logo=postgresql&logoColor=white">
</p>

<!-- demo.gif goes here once recorded -->

Herald is a small control plane and a small agent. Each service runs an agent beside it. Operators write configuration with the `herald` CLI or the HTTP API. A control-plane cluster commits each write by quorum, then pushes it to the agents whose labels match, and every change is versioned, audited and reversible.

It is written in pure Go, runs on PostgreSQL, and ships with Prometheus metrics and a provisioned Grafana dashboard.

## Why Herald

Changing a feature flag, a rate limit or a timeout usually means a redeploy, or a config service that hosts poll and nobody can audit. Both are slow when something is on fire, and neither answers the questions that matter afterwards: who changed it, what was the old value, which hosts have the new one, and how do I put it back?

Herald treats configuration as versioned, replicated data.

- **One write, every matching host.** Agents hold a live stream open, so a committed change is pushed to them instead of waiting for the next poll.
- **Committed by quorum.** Nothing is pushed until a majority of control-plane nodes acknowledge the write. A write that cannot reach quorum stays `PENDING` and is never delivered.
- **Every change has a history.** Versions form a chain, each carries a vector clock, and each write is recorded in an audit log that the database itself refuses to update or delete.
- **Rollback is just another write.** Rolling back republishes an earlier version through the same path, so it is replicated, pushed and audited like any other change.
- **Targeted rollouts.** Label selectors such as `env=prod,region=us-east` send a change to a subset of agents.
- **Local reads.** Applications read from their agent's in-memory cache, so a read never leaves the host.

### What Herald is not

- It is not a secrets manager. Values are JSON stored as plain text in PostgreSQL.
- The HTTP API has no authentication yet, so run it on a trusted network.
- It is not a full Raft implementation. Leader election uses a PostgreSQL advisory lock, and all nodes share one database. [Architecture](docs/architecture.md#where-herald-differs-from-a-textbook-design) spells out exactly what that means.

## Features

| Area                 | What you get                                                                                                                                                                               |
| -------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| **Replication**      | Leader election through a PostgreSQL advisory lock with automatic takeover. Writes replicate to peers over gRPC `AppendEntries` and commit on quorum (2 of 3 by default)                   |
| **Versioning**       | Every write is an immutable version linked to its predecessor, stamped with a per-key vector clock. Diff any two versions                                                                  |
| **Delivery**         | Server-sent-event push to agents, per-agent buffering, duplicate suppression by version ID, and a per-agent, per-version ack record. Watch a key live from the CLI                         |
| **Targeting**        | Namespaces, plus exact-match label selectors (`env=prod,region=us-east`) that decide which agents receive a change                                                                         |
| **Safety**           | Roll back to any earlier version with one command. An append-only audit log records the actor, the old value and the new value of every write and rollback, enforced by a database trigger |
| **Fleet visibility** | Agents report `HEALTHY`, `STALE` (no heartbeat for 1.5 s) or `DEAD` (5 s), together with the version they last applied. `herald status` shows the whole fleet                              |
| **Observability**    | Prometheus `/metrics` on every node, six Herald-specific metrics, a provisioned Grafana dashboard and structured JSON logs                                                                 |
| **Operations**       | Migrations run on startup. One `docker compose up` starts a three-node cluster with PostgreSQL, Prometheus and Grafana. Pure Go, built with `CGO_ENABLED=0`                                |
