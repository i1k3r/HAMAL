<div align="center">

<img src="brand/hamal-logo-light.svg#gh-light-mode-only" alt="HAMAL Logo Light" width="320" />
<img src="brand/hamal-logo-dark.svg#gh-dark-mode-only" alt="HAMAL Logo Dark" width="320" />

# HAMAL

**Fast, private, temporary file and clipboard transfer.**

[![Go Version](https://img.shields.io/badge/Go-1.23+-00ADD8?style=flat-square&logo=go)](https://golang.org)
[![Docker](https://img.shields.io/badge/Docker-Ready-2496ED?style=flat-square&logo=docker)](Dockerfile)
[![Unraid](https://img.shields.io/badge/Unraid-Compatible-E16527?style=flat-square)](unraid-template.xml)
[![License](https://img.shields.io/badge/License-Source--Available-blue.svg?style=flat-square)](LICENSE)

</div>

---

## What is HAMAL?

**HAMAL** (Turkish for *porter* / *carrier*) is a lightweight, self-hosted web application for temporary file and text transfers between devices on the same LAN / Wi-Fi network.

The idea is simple: create a temporary room, share the generated QR code or local link, and move digital cargo between devices without a cloud upload, account system, advertising, or telemetry.

HAMAL is built around short-lived transfer rooms rather than permanent storage. Rooms expire automatically, and their temporary files and text follow the room lifecycle.

### How it works

1. Create a temporary room.
2. Optionally protect it with a PIN.
3. Scan the QR code or open the local room link.
4. Transfer files or send temporary text between connected devices.
5. Let the room expire automatically, or close it manually from the creator dashboard. Only the room creator can close a room.

---

## Screenshots

HAMAL uses the **Light Table** design throughout, from opening a room to joining one and opening a public share link: a darkroom frame around a lit table, where photos lie as prints, documents and audio as manila slips, and notes as index cards. The lamp at the top shows how long the room has left. Screens below are captured from the running application with demo data.

<div align="center">

### Open a room — desktop and phone
<img src="docs/screenshots/home-desktop.jpg" alt="HAMAL home page: Open a room, with how long the room stays open, an optional PIN and the Open the room button" width="640" />
<img src="docs/screenshots/home-phone.jpg" alt="HAMAL home page on a phone with the room form in the first screen" width="250" />

<br/><br/>

### Room — creator, desktop
<img src="docs/screenshots/creator-desktop.jpg" alt="HAMAL creator view: files and notes on the lit table, lamp gauge, device chip, Show QR and Close room" width="900" />

<br/><br/>

### Inviting a device — QR projection
<img src="docs/screenshots/qr-desktop.png" alt="HAMAL QR projection: a large QR code with the room address and a Copy address button" width="900" />

<br/><br/>

### Notes and the composer
<img src="docs/screenshots/note-desktop.jpg" alt="HAMAL note composer open below the table, with notes shown as ruled index cards" width="900" />

<br/><br/>

### List view and media viewer
<img src="docs/screenshots/list-desktop.jpg" alt="HAMAL List view: the same files and notes as rows with Select all and Save selected" width="440" />
<img src="docs/screenshots/viewer-desktop.jpg" alt="HAMAL media viewer showing a photo with Previous, Save and Next" width="440" />

<br/><br/>

### Public share link — desktop and phone
<img src="docs/screenshots/share-desktop.jpg" alt="HAMAL public share page: the shared file as a slip with its type, name and size, a Download button and the time the link stops working" width="640" />
<img src="docs/screenshots/share-phone.jpg" alt="HAMAL public share page on a phone" width="250" />

<br/><br/>

### Phone — joining with a PIN, the table, and the last two minutes
<img src="docs/screenshots/pin-phone.jpg" alt="HAMAL PIN screen on a phone" width="250" />
<img src="docs/screenshots/participant-phone.jpg" alt="HAMAL participant view on a phone with Add files and Write a note" width="250" />
<img src="docs/screenshots/closing-phone.jpg" alt="HAMAL phone view in the last two minutes: warning banner and dimmed table" width="250" />

</div>

---

## Key Features

- 📋 **Temporary Text / Clipboard**: Send plain text, commands, logs, JSON/YAML, URLs, configuration snippets, and other technical text through the same temporary room.
- 🔄 **Two-Way CLIENT ↔ CLIENT Text Sharing**: Both devices can send text through the same room. Messages are labelled **CLIENT** and system activity is labelled **SERVER**.
- 📎 **One-Click Copy**: Every shared client message can be copied as the original plain text, preserving multiline content and formatting.
- 🚀 **Zero Setup for Participants**: Scan a QR code or open a local link to immediately upload/download files or use the temporary Text / Clipboard channel.
- ⏱️ **Auto-Expiring Rooms**: Rooms automatically expire and clean up temporary content after a configurable TTL.
- 🔒 **PIN Protection**: Optional 4–8 digit PIN with exponential backoff and lockout to prevent brute-force attacks.
- 📦 **True Streaming I/O**: Multi-gigabyte transfers stream directly to disk without exhausting server RAM.
- 🎨 **Responsive HAMAL UI**: Dark-first interface with warm amber accents, plus a preserved light theme for users who prefer it.
- 🔍 **Interactive QR Lightbox**: One-click smooth zoom for scanning QR codes from across the room.
- 🛡️ **Self-Hosted & Private**: Zero cloud dependencies, zero external analytics, zero tracking. All assets, including fonts, are served by your own instance; pages make no third-party requests.

Text messages are temporary and scoped to their room. They are removed with the room lifecycle; the text channel is a technical clipboard rather than a permanent chat application. Each message is limited to 64 KB (`LAN_DROP_MAX_TEXT_SIZE`), with up to 200 messages per room (`LAN_DROP_MAX_TEXTS_PER_ROOM`). System messages (labelled **SERVER**) share that capacity but never block user text: when a room is full, the oldest system messages are removed first. Text is stored exactly as sent, including leading indentation.

### Security model

- Rooms are protected by unguessable links. The creator link controls the room; the participant link (the QR code) can upload, download and exchange text, but cannot close the room.
- The optional PIN is 4–8 digits. Until a participant has entered it, files, text and the list of connected devices are not disclosed.
- HAMAL does **not** encrypt files or text at rest, and it serves plain HTTP. On an untrusted network, put it behind an HTTPS reverse proxy. Files live under `/data` until their room expires or is closed.

---

## Quick Start

### Using Docker Compose (Recommended)

1. Clone the repository:
   ```bash
   git clone https://github.com/i1k3r/HAMAL.git
   cd HAMAL
   ```

2. Copy the sample environment file:
   ```bash
   cp compose.example.env .env
   ```

3. Start the container:
   ```bash
   docker compose up -d
   ```

4. Open your browser and navigate to:
   ```
   http://localhost:7700
   ```

---

### Unraid Installation

1. Place [`unraid-template.xml`](unraid-template.xml) into `/boot/config/plugins/dockerMan/templates-user/` on your Unraid flash drive.
2. Go to the **Docker** tab in Unraid and click **Add Container**.
3. Select **HAMAL** from the template list.
4. Set your preferred web UI port and appdata directory path (`/mnt/user/appdata/hamal`), then click **Apply**.

No permission setup is needed. On first start, a new empty appdata folder is handed to Unraid's `nobody:users` (99:100); only that folder itself, never its contents.

### Data folder permissions

The container starts as root only to choose a non-root identity, then runs HAMAL (and its health check) without root, keeping `--read-only` and `no-new-privileges`:

- **Existing data:** HAMAL runs as the owner of the data it finds (for example 10001:10001 from older images and named volumes, or 99:100 on Unraid). Existing files are never chowned.
- **New, root-owned folder** (what Docker creates for a missing host path): only that directory is given to 99:100.
- **`--user`:** if you start the container with `--user`, it runs as that user exactly as before.
- **`PUID` / `PGID`** (optional): force a specific UID and/or GID. Each one replaces only its own half (PUID alone keeps the detected GID, PGID alone keeps the detected UID). Values must be whole numbers from 1 to 4294967294; 0 (root) is refused.
- **`LAN_DROP_DATA_DIR` / `LAN_DROP_DB_PATH`** are honoured the same way as by the server. A missing data directory is created for the runtime user; the database's directory must already exist.

If something is still not writable, the log lists each path with its owner and mode instead of a bare "permission denied".

---

### Manual Binary Build (Go)

Prerequisites: Go 1.23+, GCC / CGO (for SQLite support).

```bash
# Clone the repository
git clone https://github.com/i1k3r/HAMAL.git
cd HAMAL

# Run tests
go test ./...

# Build the executable
go build -o hamal ./cmd/lan-drop

# Run HAMAL
./hamal
```

---

## Configuration

HAMAL is configured via environment variables:

| Variable | Default | Description |
| :--- | :--- | :--- |
| `LAN_DROP_LISTEN_ADDR` | `:7700` | Address and port to bind the HTTP server |
| `LAN_DROP_DATA_DIR` | `/data` | Root directory for SQLite DB, temporary files, and secrets |
| `PUID` / `PGID` | *(auto)* | Docker image only, optional. UID / GID to run as (1–4294967294, each independent); by default the owner of existing data, or 99:100 for a new folder |
| `LAN_DROP_DEFAULT_TTL` | `1h` | Default room lifetime |
| `LAN_DROP_MIN_TTL` | `5m` | Minimum allowed room lifetime |
| `LAN_DROP_MAX_TTL` | `24h` | Maximum allowed room lifetime |
| `LAN_DROP_MAX_FILE_SIZE` | `10737418240` (10 GB) | Maximum size of an individual file |
| `LAN_DROP_MAX_ROOM_SIZE` | `10737418240` (10 GB) | Maximum aggregate file storage per room |
| `LAN_DROP_MAX_FILES_PER_ROOM`| `100` | Maximum number of files per room |
| `LAN_DROP_MAX_TEXT_SIZE` | `65536` (64 KB) | Maximum size of a single text message; the web UI uses the same limit |
| `LAN_DROP_MAX_TEXTS_PER_ROOM` | `200` | Maximum number of text messages kept per room |
| `LAN_DROP_CLEANUP_INTERVAL` | `1m` | Frequency of background room and orphan file sweeps |
| `LAN_DROP_LOG_FORMAT` | `json` | Log format (`json` or `text`) |
| `LAN_DROP_LOG_LEVEL` | `info` | Log verbosity (`debug`, `info`, `warn`, `error`) |
| `LAN_DROP_SECURE_COOKIES` | `auto` | Cookie security (`auto`, `true`, `false`) |
| `LAN_DROP_TRUSTED_PROXIES` | *(empty)* | Comma-separated trusted proxy IP addresses or CIDR ranges (for example `192.168.1.5,10.0.0.0/8`). When using an HTTPS-terminating reverse proxy, configure this or set `LAN_DROP_SECURE_COOKIES=true` so session cookies receive the `Secure` attribute. |

---

## Official Desktop Applications

HAMAL is available as a native, zero-cloud desktop application for multiple platforms:

- **[HAMAL for Windows](https://github.com/i1k3r/HAMAL-Windows)**: Native Windows desktop client packaged with MSIX and Microsoft Store distribution support.
- **[HAMAL for Linux](https://github.com/i1k3r/HAMAL-Linux)**: Native Linux desktop application and Snap package with WebKit2GTK engine and 20-language support.

---

## Brand Assets

Official HAMAL vector logos and icons are available in the [`brand/`](brand/) directory:

- `brand/hamal-logo-dark.svg` (Horizontal dark-mode wordmark)
- `brand/hamal-logo-light.svg` (Horizontal light-mode wordmark)
- `brand/hamal-logo-stacked.svg` (Stacked badge wordmark)
- `brand/hamal-app-icon.svg` (Square container icon)
- `brand/hamal-monochrome.svg` (Monochrome asset)
- `brand/hamal-path-mark.svg` (Vector path symbol)

---
## License

HAMAL is source-available software licensed under the
[HAMAL Source-Available Organizational Use License](LICENSE).

Organizations may freely use, modify, fork, and deploy HAMAL for their own
internal operations, including commercial business operations.

Commercial redistribution, resale, productization, and offering HAMAL or
derivative works as a commercial service require separate permission from the copyright holder.

The bundled UI fonts (Inter, JetBrains Mono and Caveat) are third-party works licensed under the
SIL Open Font License 1.1; their license texts are in
[`internal/app/static/fonts/`](internal/app/static/fonts/).