# VPSMonitor v0.2.67

## Changes
- Added 3x-ui v3 client API compatibility: client collection now reads `/panel/api/clients/list` and merges clients back into each inbound for existing overview, authorization, and export flows.
- Updated x-ui client actions to prefer v3 endpoints (`/panel/api/clients/add`, `/get`, `/update`, `/del`) while preserving legacy fallback behavior.
- Added 1Panel Docker 3x-ui database fallback path: `/opt/1panel/docker/compose/3x-ui/db/x-ui.db`.
- Realm forwarding target selection now prefers the target VPS primary/import domain over IP when filling the target address.

## Validation
- `go test ./...`
- `npm run build`

## Assets
- VPSMonitor-client-linux-amd64.tar.gz
- VPSMonitor-client-windows-amd64.zip
- VPSMonitor-server-linux-amd64.tar.gz
- VPSMonitor-server-windows-amd64.zip
- checksums.txt
