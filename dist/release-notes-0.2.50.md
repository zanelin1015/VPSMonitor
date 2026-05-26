## v0.2.50

- 修复新版 3x-ui v3 数据库移除 `all_time` 字段后，Client 采集节点报错的问题。
- x-ui 采集链路调整为优先调用面板 API，失败后再回退读取本地 SQLite DB。
- API token 模式支持 3x-ui v3 `Authorization: Bearer <token>` 认证。
- 本地 DB 回退读取改为动态检查字段，缺失 `all_time` 等字段时使用默认值，提升 v3 schema 兼容性。
- 增加 API 优先与 v3 DB schema 兼容测试。
