## VPSMonitor 0.3.2

### X-UI API Token 临时下发
- API Token 继续加密保存在 Server，每次 X-UI 面板操作执行时读取最新值并临时下发给对应 Client。
- WebSocket 实时动作和 Client 离线后的轮询动作均支持临时 Token，不再依赖 Client 中已缓存的认证对象。
- Client 为每次携带 Token 的动作创建一次性 X-UI 连接，用完即释放；更新 Token 后无需重启 Client。
- Token 不写入动作表、动作 payload、动作历史或管理员动作列表。
- 新旧 Server/Client 保持协议兼容，升级 Client 后旧进程内存中的 Token 会随服务重启清除。

### Clash / Mihomo 订阅名称
- 下载订阅的名称改为 Customer 用户名，例如用户名 `TT` 对应订阅名称 `TT`。
- 移除 `-mihomo.yaml` 显示后缀，并修复 Clash Verge 将响应头双引号显示为 `\\"` 的兼容问题。
- UTF-8 用户名通过标准扩展文件名参数传递，ASCII 回退文件名保持安全。

### 验证
- Go 全量测试与 Go Vet 通过。
- 覆盖 Token 实时下发、轮询下发、不落库、一次性连接和最新 Token 覆盖旧 Token 的测试。
- 前端 TypeScript 检查、生产构建、客户端到期与财务回归测试通过。
- Linux / Windows Server 与 Client 0.3.2 安装包生成成功。
