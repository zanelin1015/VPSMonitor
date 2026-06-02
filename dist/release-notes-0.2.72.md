# VPSMonitor v0.2.72

## Changes
- Update the top overview cards to focus on enabled capabilities: x-ui node/client counts, realtime speed, Realm forwarding count, port policy count, and used traffic chart.
- Hide redundant Client / node header tags and "view clients" link above each node row in the node tab.
- Adjust x-ui action restart behavior: client add/update/enable/delete actions no longer restart x-ui or Xray; outbound/routing forwarding rule changes still restart Xray.
- For Docker-hosted x-ui, Xray restart now only uses the x-ui panel API and no longer falls back to restarting host x-ui services.
- Fix client billing start date handling: the selected date is treated as the period start, and x-ui expiry follows the billing cycle instead of being set to the start date.
- Add regression coverage for quarterly billing start-date expiry calculation.

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
