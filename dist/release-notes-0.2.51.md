## v0.2.51

- 总览新增 VPS 功能开关，支持按 Client 显示/隐藏 x-ui、Realm、NAT、端口限速相关 Tab。
- 兼容已存在的老 Client：未显式保存功能开关时，会根据现有 x-ui、Realm、NAT、端口限速配置和最新快照自动推断开启状态。
- x-ui 客户端列表新增启用 / 停用能力，支持通过 WS 实时下发到 Client，并调用 3x-ui API 更新客户端状态后重启 Xray。
- 区域账号的 x-ui 客户端启停继续按已授权节点/客户端范围校验，不放开整机权限。
- Realm 转发新增“复制到 Client 并生效”：可将当前 Client 的有效 Realm 配置复制到另一台 Client，并立即通知目标 Client 应用。
- Realm 配置复制会自动启用目标 Client 的 Realm 功能开关，并保留配置审计记录。
