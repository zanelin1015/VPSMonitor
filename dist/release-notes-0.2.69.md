# VPSMonitor v0.2.69

## Changes
- Move x-ui and Realm matched node-client hierarchy into the node tabs, showing each VPS/Client -> node -> clients structure.
- Keep client tabs as flat client lists only; adding clients is now only available from node rows.
- Include Realm-forwarded nodes in overview data with target agent/inbound metadata so Realm-only entries can add clients on the matched target x-ui node.
- Preserve Realm target agent names on forwarded clients and nodes for non-admin/area-manager views.

## Validation
- `./scripts/build.sh`
- `go test ./...`
- `npm run build`

## Assets
- VPSMonitor-client-linux-amd64.tar.gz
- VPSMonitor-client-windows-amd64.zip
- VPSMonitor-server-linux-amd64.tar.gz
- VPSMonitor-server-windows-amd64.zip
- checksums.txt
