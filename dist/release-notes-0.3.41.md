# VPSMonitor v0.3.41

## 更新内容

- 客户 Clash/Mihomo 订阅配置头与第三方配置对齐。
- 使用独立的 HTTP `7890` 和 SOCKS5 `7891` 端口，并显式启用局域网绑定。
- 控制器 API 改为仅监听本机 `127.0.0.1:9090`。
- DNS 配置增加 `0.0.0.0:53` 监听、Fake-IP 排除列表、国内/国外 DNS 分流及 fallback 策略。
- 保留 VPSMonitor 动态生成的代理节点、前置代理、代理组和路由规则。

## 验证

- Go 全量测试通过。
- 前端 TypeScript 检查和生产构建通过。
- 客户 Clash/Mihomo 订阅生成专项测试通过。
- Linux amd64 / arm64 / ARMv7 和 Windows amd64 / arm64 的 Server、Client 交叉编译通过。

## 升级注意

- Server 和 Web 必须同时升级到 v0.3.41。
- 新配置会让 Mihomo/Clash 监听本机 DNS `53` 端口，请确认系统没有其他 DNS 服务占用该端口。
- 升级前请备份 SQLite 数据库及配置。
