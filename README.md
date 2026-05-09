# Bridge Core

`bridge-core` 是一个围绕 `3x-ui` 的中心编排层，不替代 `x-ui` 本身。

- `bridge-server` 负责 client 注册、快照存储、x-ui 托管配置和 Web 控制台。
- `bridge-client` 部署在每台 VPS 上，负责连接本机 x-ui，把状态和结构信息回传给 `bridge-server`。

当前中心的定位很明确：

- 节点 / 入站仍然在各台 x-ui 面板里手动维护。
- `bridge-server` 只负责观察、汇总，以及统一编排出站和转发规则。
- 你可以把 client A 的节点客户端信息导入成 client B 的出站，再直接给 client B 下发路由规则。

服务端默认使用 `SQLite`，数据库文件默认是 `./data/bridge.db`。

## 当前架构

- 一台 `server`
- 多台 `client`
- `client` 首次启动后自动向 `server` 注册
- `server` 在数据库里保存 client 基本信息和最近快照
- `server` 后台可修改每台 client 的 x-ui 托管配置
- `client` 每次轮询前先拉取自己的托管配置，再按该配置采集和执行待处理操作

这意味着：

- `server.json` 不需要手工写一堆 agents
- `client.json` 只保留 bootstrap 配置
- 真正长期生效的 x-ui 地址、账号、密码由 `server` 托管

## 当前能力

- 自动注册 client
- 自动生成并下发 `agent token`
- 自动登录 x-ui
- 通过 x-ui HTTP 接口抓取：
  - `/panel/api/inbounds/list`
  - `/panel/api/server/status`
  - `/panel/api/server/getConfigJson`
  - `/panel/xray/getOutboundsTraffic`
- 服务端保存：
  - agent 基本信息
  - agent 最新快照
  - agent 历史快照
  - x-ui 托管配置
  - x-ui 操作记录
- Web 控制台支持：
  - 查看节点、客户端、出站、路由规则
  - 从客户端或节点追踪当前命中的出站规则
  - 从其它 client 导入节点客户端信息为当前 client 的出站
  - 为当前 client 下发出站新增和路由规则新增
  - 一键跳转到对应 client 的 x-ui 面板手动维护节点

## 目录

- [cmd/bridge-server/main.go](/Users/alan/Desktop/WorkPlace/Projects/NanFengMonitor/bridge-core/cmd/bridge-server/main.go)
- [cmd/bridge-client/main.go](/Users/alan/Desktop/WorkPlace/Projects/NanFengMonitor/bridge-core/cmd/bridge-client/main.go)
- [cmd/bridge-devseed/main.go](/Users/alan/Desktop/WorkPlace/Projects/NanFengMonitor/bridge-core/cmd/bridge-devseed/main.go)
- [cmd/bridge-devpanels/main.go](/Users/alan/Desktop/WorkPlace/Projects/NanFengMonitor/bridge-core/cmd/bridge-devpanels/main.go)
- [internal/server/app.go](/Users/alan/Desktop/WorkPlace/Projects/NanFengMonitor/bridge-core/internal/server/app.go)
- [internal/client/app.go](/Users/alan/Desktop/WorkPlace/Projects/NanFengMonitor/bridge-core/internal/client/app.go)
- [internal/store/sqlite_store.go](/Users/alan/Desktop/WorkPlace/Projects/NanFengMonitor/bridge-core/internal/store/sqlite_store.go)
- [internal/panels/xui.go](/Users/alan/Desktop/WorkPlace/Projects/NanFengMonitor/bridge-core/internal/panels/xui.go)

## 配置

- 服务端示例: [config/server.example.json](/Users/alan/Desktop/WorkPlace/Projects/NanFengMonitor/bridge-core/config/server.example.json)
- 客户端示例: [config/client.example.json](/Users/alan/Desktop/WorkPlace/Projects/NanFengMonitor/bridge-core/config/client.example.json)

### `server.json`

- `listen_addr`: 服务监听地址
- `data_dir`: 运行数据目录
- `database_path`: SQLite 文件路径，不写则默认 `data_dir/bridge.db`
- `credential_key_path`: x-ui 托管密码加密密钥文件，不写则默认 `data_dir/credential.key`
- `registration_token`: client 首次注册时使用的口令
- `admin_username`: 初始管理员用户名，仅在数据库还没有管理员账号时生效
- `admin_password`: 初始管理员密码，仅在数据库还没有管理员账号时生效
- `snapshot_retention_days`: 历史心跳快照按时间保留的天数，默认 30；设置为负数可关闭按时间清理
- `snapshot_retention_count`: 每个 client 最多保留的历史心跳快照数，默认 5000；设置为负数可关闭按数量清理
- `agents`: 兼容字段，统一模式下可以直接留空

推荐示例：

```json
{
  "listen_addr": ":8090",
  "data_dir": "./data",
  "database_path": "./data/bridge.db",
  "credential_key_path": "./data/credential.key",
  "registration_token": "replace-with-registration-token",
  "admin_username": "admin",
  "admin_password": "replace-with-admin-password",
  "snapshot_retention_days": 30,
  "snapshot_retention_count": 5000,
  "agents": []
}
```

管理员账号会写入 SQLite，密码只保存 PBKDF2-SHA256 哈希。x-ui 托管密码会使用 `credential_key_path` 中的本地密钥加密后再写入 SQLite；Web 控制台和 client 拉取配置时仍返回解密后的明文，便于查看和实际登录 x-ui。首次启动后可以在 Web 控制台里修改用户名和密码，之后不用再改 `server.json`。

### `client.json`

- `registration_token`: 用来向 server 注册
- `agent_token`: 兼容保留字段，统一注册模式下可以留空
- `server_url`: 指向中心服务地址
- `server_skip_tls_verify`: 是否跳过中心服务证书校验
- `poll_interval`: client 轮询周期
- `request_timeout_seconds`: 请求超时时间

`client.json` 不再配置 `agent_id`、`agent_name`、标签、x-ui 地址、账号和密码。client 会用本机 hostname 自动生成初始 ID 注册；展示名、标签和 x-ui 信息都在 server 控制台里维护。client 每次启动/轮询时会先向 server 注册或拉取托管配置，然后按 server 下发的 x-ui 配置采集数据和执行操作。

## 运行

### 1. 启动服务端

```bash
cd /Users/alan/Desktop/WorkPlace/Projects/NanFengMonitor/bridge-core
cp config/server.example.json config/server.json
go run ./cmd/bridge-server -config ./config/server.json
```

启动后访问：

```text
http://127.0.0.1:8090/
```

### 2. 启动客户端

```bash
cd /Users/alan/Desktop/WorkPlace/Projects/NanFengMonitor/bridge-core
cp config/client.example.json config/client.json
go run ./cmd/bridge-client -config ./config/client.json -once
```

第一次 `-once` 会执行：

1. 使用 `registration_token` 向 server 注册
2. 拿到正式 `agent_token`
3. 拿到该 agent 当前的托管配置
4. 如果 server 已配置 x-ui，则登录 x-ui 抓取概览
5. 上报快照

确认注册成功后，再跑常驻：

```bash
go run ./cmd/bridge-client -config ./config/client.json
```

### 3. 在后台修改托管配置

- 使用管理员账号登录
- 左侧选择一台 client
- 进入 `托管配置`
- 修改 x-ui 地址、账号、密码
- 保存

保存后，这台 client 下一次轮询会自动生效。

### 4. 出站和转发的工作方式

- 如果要新增节点 / 入站：直接点击控制台里的 `打开 x-ui 面板`，去原始 x-ui 页面维护
- 如果要做中转：在目标 client 上创建 `新增出站`
- 在出站表单中选择来源 client 和来源客户端，自动导入为当前 client 的 outbound
- 然后继续在当前 client 上创建 `新增转发 / 路由规则`

## 本地调试

### 方式一：直接注入演示数据

适合调 `server`、前端页面、节点/客户端/出站/路由展示。

先启动 `bridge-server`，然后执行：

```bash
go run ./cmd/bridge-devseed \
  -server http://127.0.0.1:8090 \
  -registration-token preview-registration-token \
  -agent-id local-dev-01 \
  -agent-name "Local Dev VPS"
```

执行后刷新控制台，就能看到一台模拟 VPS，里面带有：

- 入站节点
- 节点下的客户端
- 出站
- 路由规则
- 可用于“导入为出站”的示例客户端认证信息

### 方式二：启动 mock x-ui 面板

适合调 `bridge-client` 的真实采集链路，以及出站 / 路由下发链路。

先启动 mock 面板：

```bash
go run ./cmd/bridge-devpanels -listen 127.0.0.1:19090
```

然后在 Web 控制台选择该 client，进入 `托管配置`，把 x-ui 指向 mock 面板：

```json
{
  "xui": {
    "enabled": true,
    "base_url": "http://127.0.0.1:19090",
    "username": "admin",
    "password": "password",
    "skip_tls_verify": false
  }
}
```

保存后再跑一次 client：

```bash
go run ./cmd/bridge-client -config ./config/client.json -once
```

这会走完整链路：client 注册、拉托管配置、登录 mock x-ui、采集数据、上报 server。之后你也可以在控制台下发出站和路由规则，再观察 mock 面板内配置变化。

## 打包

Linux/macOS：

```bash
cd /Users/alan/Desktop/WorkPlace/Projects/NanFengMonitor/bridge-core
chmod +x ./scripts/build.sh
./scripts/build.sh
```

Windows PowerShell：

```powershell
Set-Location /Users/alan/Desktop/WorkPlace/Projects/NanFengMonitor/bridge-core
./scripts/build.ps1
```

打包需要本机具备：

- Go
- Node.js 18+
- npm

脚本会先构建前端静态资源，再把页面嵌入 `bridge-server` 二进制，并输出 Linux / Windows 可直接运行的包。
