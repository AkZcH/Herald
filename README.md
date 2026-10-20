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
