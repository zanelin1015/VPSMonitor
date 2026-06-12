# VPSMonitor v0.2.82

## 修复
- 修复系统新增节点客户端时 UUID 使用固定占位值 `00000000-0000-0000-0000-000000000001`，导致同一 3x-ui 内多个客户端 UUID 重复的问题。
- 前端新增客户端 UUID 留空时不再生成递增占位 UUID，由 client 执行下发时随机生成真实 UUID。
- client 执行 x-ui 新增客户端前会兜底替换空 UUID、全 0 UUID 和旧递增占位 UUID，同时保留已存在的真实 UUID。
- 新增入站节点时，入站内客户端列表也会统一做 UUID 随机化处理。

## 验证
- `go test ./...`
- `VPSMONITOR_BUILD_VERSION=0.2.82 ./scripts/build.sh`
