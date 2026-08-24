# HAMAL Architecture

HAMAL is a single-container, self-hosted temporary local file courier service. The architecture uses a Go server, server-rendered templates with vanilla JavaScript enhancements, SQLite metadata, and opaque filesystem storage under one persistent `/data` mount.

## Foundation scope

This phase contains application startup/shutdown, environment configuration, structured logs, request IDs, embedded templates/static assets, SQLite initialization, storage validation, health/readiness endpoints, Docker deployment, and basic tests. It deliberately excludes rooms, file transfer, QR generation, PINs, creator credentials, cleanup workers, and Global Share.

## Locked MVP decisions

- One non-root OCI container; no external database, Redis, host networking, privileged mode, or Docker socket.
- `/data` is the only required persistent mount.
- SQLite uses WAL mode, foreign keys, and a busy timeout.
- Go templates, CSS, and vanilla JavaScript are used instead of React, TypeScript, Vite, SSE, or WebSockets.
- Future room participant and creator credentials will be independent cryptographically random bearer secrets; QR codes will contain participant URLs only.
- Raw credentials, room tokens, PINs, cookies, query strings, and authorization headers must never be logged.
- Future uploads will stage beneath `/data/staging` and become visible only after atomically moving to opaque paths beneath `/data/files`.
- Global Share is a future stricter policy profile of the same self-hosted instance; it is not a hosted LAN-Drop service.

## Foundation runtime flow

1. Validate configuration.
2. Create and verify `/data`, `/data/files`, `/data/staging`, and `/data/secrets`.
3. Load an environment-provided server secret or persist a generated secret beneath `/data/secrets`.
4. Open SQLite, enable WAL/foreign keys/busy timeout, and run foundation migrations.
5. Serve the landing page, `/healthz`, and `/readyz`.
