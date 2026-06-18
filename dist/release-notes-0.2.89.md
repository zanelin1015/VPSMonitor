# VPSMonitor v0.2.89

## 测试
- 新增 admin 财务口径自检脚本，覆盖全部 VPS 开支、普通用户节点收入、未分配节点收入、区域账号整体收入。
- 覆盖区域账号开启整体计费时避免重复统计节点收入，以及关闭整体计费时节点收入正常计入。

## 验证
- `go test ./...`
- `cd web && npm run test:finance`
- `./scripts/build.sh`
