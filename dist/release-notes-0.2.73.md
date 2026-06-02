# VPSMonitor v0.2.73

## Changes
- Restore the top dashboard overview to global/workbench-style Client statistics instead of per-capability details.
- Move x-ui node/client counts, realtime speed, Realm forwarding count, port policy count, and used traffic chart into the selected Client's internal Overview tab.
- Keep the selected Client overview capability cards conditional on enabled features.

## Validation
- `./scripts/build.sh`
- `go test ./...`
- `cd web && npm run build`

## Assets
- VPSMonitor-client-linux-amd64.tar.gz
- VPSMonitor-client-windows-amd64.zip
- VPSMonitor-server-linux-amd64.tar.gz
- VPSMonitor-server-windows-amd64.zip
- checksums.txt
