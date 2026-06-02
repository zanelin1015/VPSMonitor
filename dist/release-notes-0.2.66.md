## v0.2.66

- 修复 3x-ui 部分版本或反代环境下 `/panel/xray/getOutboundsTraffic` 返回 HTML 时，client 把 HTML 当 JSON 解析导致 x-ui 采集失败的问题。
- 出站流量统计接口改为可选采集；该接口异常时不再影响节点、客户端、出站和路由规则等主数据读取。
- 增加回归测试覆盖 HTML 响应场景，确保后续兼容新版 3x-ui 或面板路径差异。
