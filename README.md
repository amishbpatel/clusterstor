# ClusterStor

ClusterStor unifies supported cloud accounts into one logical drive and adds optional ClusterStor-managed cloud storage, with opt-in Peer Storage planned as a later production subsystem.

## V1 stack
- Go backend/API and worker
- PostgreSQL 18
- SvelteKit + TypeScript web app
- Go desktop agent + watchdog
- Wails + Svelte tray/settings UI later
- Backblaze B2 behind an object-storage abstraction
- Google Drive, OneDrive, Dropbox, Box
- Stripe
- REST/JSON + OpenAPI 3.x
- WebSocket notifications + HTTPS catch-up

## Build strategy
Google Drive is the first end-to-end vertical slice. Once account -> agent -> MyDrive -> Google upload -> metadata -> second-client visibility -> download works reliably, the remaining providers are added behind the same interface.

## Quick start
```bash
cp .env.example .env
docker compose up -d postgres
go test ./...
go run ./cmd/api
```

Health: `GET http://localhost:8080/healthz`

## Product principles
- Dashboard is the default website landing page; My Files is one click away.
- Large customer file bytes should move directly between client and storage provider where practical.
- ClusterStor-managed Cloud and future Peer bytes are client-side encrypted before upload.
- Peer contribution is explicit opt-in during desktop-agent setup.
- Peer operational state moves to a separate PostgreSQL database/cluster when Peer becomes a production subsystem.
