## v0.2.57

- 托管配置保存后新增 `apply_config` WebSocket 指令，Server 会把最新配置直接推送给在线 Client。
- Client 收到 `apply_config` 后立即应用端口策略、Realm 转发等本机配置，并立刻上报快照，不再依赖下一次轮询。
- 保留 `collect_now` 兼容旧版 Client；新版 Client 会抑制紧随其后的重复采集，避免重复执行。
- 端口策略保存成功提示更新为“已通过 WS 通知 Client 立即生效”。
