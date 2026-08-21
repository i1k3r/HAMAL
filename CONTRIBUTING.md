# Contributing to LAN-Drop

Thank you for helping build LAN-Drop. Keep contributions focused, documented, tested, and consistent with the locked MVP architecture in [`ARCHITECTURE.md`](ARCHITECTURE.md).

## Development principles

- Keep the deployment model to one OCI container and one `/data` mount.
- Do not introduce a required external service without an explicit architecture discussion.
- Prefer Go standard-library facilities where they meet the need.
- Keep the frontend server-rendered with plain CSS and small vanilla JavaScript.
- Never log secrets, bearer tokens, room URLs, cookies, PINs, or authorization headers.
- Add tests for behavior changes, especially security and storage boundaries.

## Local checks

```sh
gofmt -w .
go test ./...
go vet ./...
docker build -t lan-drop:local .
```

Please open an issue or discussion before changing any locked architecture decision.
