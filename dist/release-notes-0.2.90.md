# VPSMonitor v0.2.90

## 修复
- 修复线上 `client_chains` 为空时，admin 财务收入只剩区域账号整体收入的问题。
- 收入明细现在会在链路数据缺失时从 `client_billings` 兜底统计，确保普通节点、未分配节点和区域账号收入都进入 admin 财务。
- 保持区域账号整体计费去重：开启整体计费时不重复计算其名下节点收入。

## 测试
- 补充财务测试：覆盖无链路数据时的 `client_billings` 兜底统计。

## 验证
- `go test ./...`
- `cd web && npm run test:finance`
- `./scripts/build.sh`
