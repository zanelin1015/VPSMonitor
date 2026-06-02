# VPSMonitor v0.2.70

## Changes
- Split area-manager authorization into independent x-ui and Realm selections.
- Selecting Realm ports now auto-grants the matched target x-ui node when a Realm rule points to another Client node, while still allowing manual partial client authorization.
- Allow area managers to add clients on a node when they have either full node authorization or authorization to part of that node's clients.
- Keep node tabs focused on nodes only; add a "view clients" jump that switches to the client tab and filters by the selected node.
- Hide Realm overview sections and matched Realm link details when the VPS does not have Realm enabled.

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
