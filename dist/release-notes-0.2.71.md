# VPSMonitor v0.2.71

## Changes
- Fix area-manager x-ui authorization so selecting only part of a node's clients no longer upgrades that grant into the whole node when Realm port authorization auto-detects the target node.
- Prefer exact client grants over accidental whole-node grants on the same inbound, preventing non-authorized clients from appearing to area accounts.
- Filter x-ui routing rules for area accounts; client-specific rules for unauthorized users are hidden, and mixed-user rules are sanitized to only show authorized users.
- Clear sanitized route summaries so hidden client names are not leaked through cached rule summaries.

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
