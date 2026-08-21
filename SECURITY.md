# Security Policy

## Security principles

LAN-Drop is designed as self-hosted software. Operators control their storage, deployment environment, reverse proxy, and any Internet exposure. The project will not operate central storage, accounts, relays, analytics, or advertising.

The current foundation implements a few defensive building blocks: non-root containers, a single explicit persistent storage location, SQLite WAL initialization, request IDs, structured logging, health checks, and protections against logging future sensitive room URL paths.

The application is **not yet production secure** and is **not ready for general use**. Temporary rooms, file transfer, access controls, PIN handling, rate limiting, cleanup, and Global Share have not been implemented in this phase.

## Secret handling

`LAN_DROP_SERVER_SECRET` is never logged or stored in SQLite. If it is omitted, LAN-Drop generates and stores a persistent secret at `/data/secrets/server-secret`. Operators must protect the `/data` mount and back it up according to their own operational requirements.

## Reporting a vulnerability

Until a dedicated private reporting channel is published, please do not disclose suspected vulnerabilities in public issues. Contact the repository maintainer privately through the contact method listed on the GitHub profile for `i1k3r`, including reproduction details and impact.
