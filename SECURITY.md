# Security Policy

## Security Principles

HAMAL is designed as private, self-hosted file transfer software. Operators control their storage, deployment environment, reverse proxy, and any network exposure. The project does not operate central storage, accounts, relays, analytics, telemetry, or advertising.

Core security features include:
- Ephemeral, time-limited room lifecycles with automatic purge upon expiry.
- Ephemeral cryptographic tokens separating creator controls from participant access. Closing a room is a creator-only action.
- Optional 4–8 digit PIN protection with exponential rate limiting and lockout. Files, text and the list of connected devices are disclosed only after PIN authentication.
- Pure local network streaming without cloud relay or persistent staging retention.
- Zero credential logging: tokens, PINs, and sensitive authorization headers are never logged.

## What HAMAL does not do

- Files and text are **not encrypted at rest**; they are stored as plain files and SQLite rows under `/data` until the room expires or is closed.
- HAMAL serves plain HTTP. Traffic is encrypted in transit only if the operator places it behind an HTTPS reverse proxy.
- Room links are bearer credentials: anyone who obtains a creator or participant link has that role's access.

## Secret Handling

`LAN_DROP_SERVER_SECRET` (or generated fallback) is never logged or stored in SQLite. If omitted, HAMAL generates and stores a persistent server secret at `/data/secrets/server-secret`. Operators must protect the `/data` mount according to their operational requirements.

## Reporting a vulnerability

Until a dedicated private reporting channel is published, please do not disclose suspected vulnerabilities in public issues. Contact the repository maintainer privately through the contact method listed on the GitHub profile for `i1k3r`, including reproduction details and impact.
