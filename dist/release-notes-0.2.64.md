## v0.2.64

- 修复区域账号在 Realm 入口页给已授权 HK x-ui 节点新增客户端时，可能被后端误判为 `agent is not assigned to this account` 的问题。
- 区域账号权限校验现在会同时参考显式授权的 x-ui 节点/客户端 assignment，避免仅依赖登录会话中的 agent 列表。
- 增加回归测试覆盖：区域账号会话 agent 列表滞后时，仍可对已授权的 HK 节点执行新增客户端操作。
