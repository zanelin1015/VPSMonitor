# VPSMonitor

VPSMonitor 是一套围绕 `x-ui / 3x-ui` 的 VPS 集中管理系统。它不替代每台机器上的 x-ui 面板，而是在其上提供统一的 Client 注册、监控、配置编排、链路拓扑、客户订阅和运维入口。

项目由两部分组成：

- `bridge-server`：中心服务、SQLite 数据库和内嵌 Web 管理台。
- `bridge-client`：部署在每台 VPS 或 iStoreOS/OpenWrt 上，采集本机状态、连接本机 x-ui，并执行 Server 下发的任务。

当前版本的默认数据文件是 `./data/bridge.db`，生产环境建议把 `data` 放在独立且可备份的持久化目录中。

## 能做什么

管理端可以完成：

- Client 自动注册、在线状态、CPU/内存/磁盘、网速和流量监控。
- x-ui 节点、客户端、出站和路由规则查看与下发；路由规则支持单条和多选删除，并保护系统 API 路由。
- 按 VPS、节点和客户端名称搜索定位，支持跨列表分页查找。
- 从一台 Client 导入另一台 Client 的节点客户端，配置 Realm 中转或 HAProxy 主备转发。
- 拓扑图和 Client 链路追踪；工作台和 Client 页面会对同一条转发链路去重，避免网速与流量重复累加。
- Client 收费周期、流量上限、收入、成本和利润统计。
- 客户账号、节点授权、Clash/Mihomo 订阅、前置代理组和在线客服。
- 客户公告历史、按账号的已读状态和访问日志排查。
- Server 和 Client 的 GitHub Release 检查、校验和升级。Client 可以在管理台中逐台选择升级。

节点和入站仍在对应的 x-ui 面板中维护。VPSMonitor 通过 x-ui API 读取和编排数据；需要新增入站时，从 Client 详情页打开对应的 x-ui 面板即可。

## 架构与注册流程

```text
bridge-client  -- HTTPS polling / realtime -->  bridge-server
      |                                             |
   本机 x-ui API                              SQLite + Web 管理台
```

Client 第一次启动时使用 Server 的共享 `registration_token` 注册，Server 返回该 Client 专属的 `agent_token`。Client 会把专属 Token 写回 `client.json`，后续请求使用专属 Token；共享注册 Token 只用于首次注册。

Server 会保存 Client 基本信息、最近快照、历史快照、x-ui 托管配置、操作记录、访问日志和客户公告已读记录。x-ui 密码使用 `credential_key_path` 指定的本地密钥加密后写入 SQLite，数据库和密钥文件需要一起备份。

## 生产环境安装

安装器会自动识别 `linux/amd64`、`linux/arm64` 和 ARMv7，并按系统使用 systemd 或 OpenRC。Server、普通 Linux Client 和 iStoreOS/OpenWrt Client 使用不同的服务管理器。

### 安装 Server

```bash
curl -fsSL https://raw.githubusercontent.com/zanelin1015/VPSMonitor/main/install.sh \
  -o /tmp/vpsmonitor-install.sh
chmod +x /tmp/vpsmonitor-install.sh
sudo /tmp/vpsmonitor-install.sh server
```

安装器会依次询问监听地址、数据目录、TLS 证书和私钥、管理员账号密码以及 Client 注册 Token。默认安装目录是 `/opt/vpsmonitor/server`，服务名是 `vpsmonitor-server`。

生产环境必须满足以下任一条件：

- 直接在 `server.json` 同时配置 `tls_cert_file` 和 `tls_key_file`。
- 使用 HTTPS 反向代理终止 TLS，并在 `trusted_proxy_cidrs` 中填写反向代理的 IP/CIDR。

`allow_insecure_http` 只适合本机开发。不要为了绕过证书错误在生产 Client 上设置 `server_skip_tls_verify: true`。

### 安装普通 Linux Client

在目标 VPS 上执行，使用 Server 安装时生成的注册 Token：

```bash
curl -fsSL https://raw.githubusercontent.com/zanelin1015/VPSMonitor/main/install.sh \
  -o /tmp/vpsmonitor-install.sh
chmod +x /tmp/vpsmonitor-install.sh

sudo env \
  VPSMONITOR_SERVER_URL="https://monitor.example.com" \
  VPSMONITOR_REGISTRATION_TOKEN="替换为Server注册Token" \
  VPSMONITOR_AGENT_ID="可选的Client_ID" \
  /tmp/vpsmonitor-install.sh client
```

不设置 `VPSMONITOR_AGENT_ID` 时，安装器会根据主机名生成 Client ID。已有 `client.json` 的升级会保留原配置；需要重新写入连接参数时，再增加 `VPSMONITOR_FORCE_CONFIG=true`。

升级到指定 Release 时设置版本即可：

```bash
sudo env VPSMONITOR_VERSION=v0.3.31 \
  VPSMONITOR_SERVER_URL="https://monitor.example.com" \
  VPSMONITOR_REGISTRATION_TOKEN="替换为Server注册Token" \
  /tmp/vpsmonitor-install.sh client
```

### 安装 iStoreOS / OpenWrt Client

在路由器 SSH 中使用 root 执行：

```sh
uclient-fetch -O /tmp/vpsmonitor-install-openwrt.sh \
  https://raw.githubusercontent.com/zanelin1015/VPSMonitor/main/install-openwrt.sh
chmod +x /tmp/vpsmonitor-install-openwrt.sh

VPSMONITOR_SERVER_URL="https://monitor.example.com" \
VPSMONITOR_REGISTRATION_TOKEN="替换为Server注册Token" \
sh /tmp/vpsmonitor-install-openwrt.sh client
```

如果系统没有 `uclient-fetch`，安装器也会尝试 `wget` 或 `curl`。服务由 procd 管理，查看状态和日志：

```sh
/etc/init.d/vpsmonitor-client status
logread -f -e vpsmonitor-client
```

### 安装 Windows Client

在管理员 PowerShell 中执行：

```powershell
$env:VPSMONITOR_SERVER_URL = "https://monitor.example.com"
$env:VPSMONITOR_REGISTRATION_TOKEN = "替换为Server注册Token"
$script = Join-Path $env:TEMP "vpsmonitor-install.ps1"
Invoke-WebRequest https://raw.githubusercontent.com/zanelin1015/VPSMonitor/main/install.ps1 -OutFile $script
powershell -NoProfile -ExecutionPolicy Bypass -File $script client
```

Windows 服务默认名为 `VPSMonitorClient`。安装器会保留已有 `client.json`，日志可以从事件查看器或服务管理器查看。

## 配置

示例文件：

- [config/server.example.json](config/server.example.json)
- [config/client.example.json](config/client.example.json)

### Server 配置

| 字段 | 作用 |
| --- | --- |
| `listen_addr` | HTTP 服务监听地址，例如 `:8090`。 |
| `tls_cert_file` / `tls_key_file` | Server 直接提供 HTTPS 时的证书和私钥，必须同时配置。 |
| `trusted_proxy_cidrs` | HTTPS 由反向代理终止时，允许传递 `X-Forwarded-Proto: https` 的代理网段。 |
| `public_path_prefixes` | 按域名启用 `/zanelin` 子路径，例如 `{"monitor.example.com":"/zanelin"}`；仅接受可信代理传入的匹配 `X-Forwarded-Prefix`。默认空对象，保留原根路径入口。 |
| `allow_insecure_http` | 仅本地开发时允许 HTTP；生产环境保持 `false`。 |
| `data_dir` | 运行数据目录，默认 `./data`。 |
| `database_path` | SQLite 路径，默认 `$data_dir/bridge.db`。 |
| `credential_key_path` | x-ui 密码加密密钥，默认 `$data_dir/credential.key`。 |
| `registration_token` | Client 首次注册使用的共享 Token。 |
| `admin_username` / `admin_password` | 仅首次初始化管理员时使用，之后以数据库中的账号为准。 |
| `snapshot_retention_days` | 历史快照按时间保留天数，默认 30。负数关闭时间清理。 |
| `snapshot_retention_count` | 每个 Client 最多保留的历史快照数，默认 5000。负数关闭数量清理。 |

Server 管理台地址通常是 `https://你的域名/`。Customer 入口是 `https://你的域名/customer`，公开站点入口是 `https://你的域名/site`（如果启用）。

使用 `/zanelin` 子路径时，需要在反向代理中同时配置路由：`/zanelin/monitor` 转到 Server 的 `/`，`/zanelin/customer` 转到 `/customer`，API 和静态资源去掉 `/zanelin` 前缀后转发，并传入 `X-Forwarded-Prefix: /zanelin`。Server 的 `public_path_prefixes` 必须明确包含该域名，`trusted_proxy_cidrs` 必须包含反向代理地址；仅添加配置字段不会自动建立这些入口。登录 Cookie、实时 WebSocket 和 Customer 订阅链接会使用同一前缀，未配置的域名继续使用原入口。

### Client 配置

```json
{
  "agent_id": "可选，首次启动时自动生成",
  "registration_token": "首次注册 Token",
  "agent_token": "首次注册成功后由 Server 写入",
  "server_url": "https://monitor.example.com",
  "server_skip_tls_verify": false,
  "poll_interval": "30s",
  "request_timeout_seconds": 15
}
```

Client 的 x-ui 地址、账号和密码不再写在本地 `client.json`。管理员在管理台的 Client 详情页进入“托管配置”维护；Client 会在下一次轮询时拉取配置。

## 手动运行与本地开发

项目要求 Go 1.25+、Node.js 18+ 和 npm。以下命令在项目根目录执行：

```bash
cp config/server.example.json config/server.json
# 本地开发时可将 allow_insecure_http 设为 true
go run ./cmd/bridge-server -config ./config/server.json
```

默认访问 `http://127.0.0.1:8090/`。启动 Client 前，把 `config/client.example.json` 复制为 `config/client.json`，填入 Server 地址和注册 Token：

```bash
cp config/client.example.json config/client.json
go run ./cmd/bridge-client -config ./config/client.json -once
go run ./cmd/bridge-client -config ./config/client.json
```

`-once` 用于首次注册和排查连接；常驻运行会按 `poll_interval` 上报快照，并每 2 秒推送实时指标。查看版本：

```bash
./bridge-server --version
./bridge-client --version
```

演示数据和 mock x-ui：

```bash
go run ./cmd/bridge-devseed \
  -server http://127.0.0.1:8090 \
  -registration-token preview-registration-token \
  -agent-id local-dev-01 \
  -agent-name "Local Dev VPS"

go run ./cmd/bridge-devpanels -listen 127.0.0.1:19090
```

然后在管理台的 Client“托管配置”中填写：

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

## Realm、HAProxy 和访问日志

Linux 与 OpenWrt 安装器可以按需安装 Realm 或 HAProxy，二者不能同时自动安装。默认都关闭：

```bash
VPSMONITOR_REALM_AUTO_INSTALL=true
VPSMONITOR_REALM_VERSION=v2.9.4
VPSMONITOR_REALM_DOWNLOAD_BASE_URL=https://mirror.example.com/realm/v2.9.4
VPSMONITOR_HAPROXY_AUTO_INSTALL=false
```

Realm 和 HAProxy 的规则由 Server 下发，Client 负责加载并上报运行状态。OpenWrt 不会自动修改 firewall4、SQM、Cake 或 qosify 规则。

访问日志需要在对应 Client 的“托管配置”中启用 x-ui access log，并填写 Client 实际可读的日志路径；例如：

```json
{
  "access_log_enabled": true,
  "access_log_path": "/var/log/xray/access.log",
  "access_log_retention_days": 7
}
```

日志由 Client 读取后上传到 Server，管理员在“访问日志”页面按 Client、来源 IP 或目标过滤。日志文件不存在、权限不足或路径填写错误时，页面不会产生记录，应先查看 Client 服务日志。

## 在线升级

管理台“个人中心 → 在线升级”会从官方 GitHub Release 获取版本和 SHA-256 校验值：

1. 先检查最新版本。
2. Server 可以升级当前服务，完成后自动重启。
3. Client 可以按系统和架构筛选，并逐台选择升级。

升级会保留 Server 的 `server.json`、SQLite 数据库和 `data`，也会保留 Client 的 `client.json`。0.3.24 及更早的旧 Client 不支持专属 Agent Token，需要先在目标机器手动升级一次到 0.3.25 或更高版本，之后才能使用管理台的校验升级。

发布新版本时，必须把 Server/Client 对应架构的安装包和 `checksums.txt` 一起上传到 GitHub Release；在线升级只接受官方仓库和带 SHA-256 的 Release 资产。

路由规则删除要求 Server 和目标 Client 均升级到 0.3.31 或更高版本。删除前会校验完整规则列表及所选规则，规则发生变化时拒绝删除并要求刷新；多选按索引倒序逐条移除，保存后重载 Xray。系统 API 路由不可删除。如果提示规则已保存但重载失败，请手动重启 x-ui / Xray，不要重复提交删除。

## 打包与测试

Linux/macOS：

```bash
chmod +x ./scripts/build.sh
./scripts/build.sh
```

指定版本：

```bash
VPSMONITOR_BUILD_VERSION=0.3.31 ./scripts/build.sh
```

脚本会先构建 `web` 前端、运行 `go test ./...`，然后输出：

- `dist/VPSMonitor-server-linux-amd64.tar.gz`
- `dist/VPSMonitor-server-linux-arm64.tar.gz`
- `dist/VPSMonitor-server-linux-arm.tar.gz`（ARMv7）
- `dist/VPSMonitor-server-windows-amd64.zip`
- `dist/VPSMonitor-server-windows-arm64.zip`
- 对应的五种 Client 安装包

Windows PowerShell：

```powershell
$env:VPSMONITOR_BUILD_VERSION = "0.3.31"
./scripts/build.ps1
```

前端单独构建和回归测试：

```bash
cd web
npm install
npm run build
npm run test:client-expiry
npm run test:dashboard-network
npm run test:finance
npm run test:agent-search
npm run test:routing-delete
npm run test:app-base
cd ..
go test ./...
```

## 目录

- [cmd/bridge-server/main.go](cmd/bridge-server/main.go)：Server 入口。
- [cmd/bridge-client/main.go](cmd/bridge-client/main.go)：Client 入口。
- [internal/server](internal/server)：HTTP API、认证、管理台和 Customer API。
- [internal/client](internal/client)：采集、x-ui、Realm、HAProxy、访问日志和升级逻辑。
- [internal/store](internal/store)：SQLite 持久化。
- [internal/dashboard](internal/dashboard)：工作台、拓扑和链路汇总。
- [web/src](web/src)：React 管理台和 Customer 页面。
- [install.sh](install.sh)：普通 Linux Server/Client 安装器。
- [install-openwrt.sh](install-openwrt.sh)：iStoreOS/OpenWrt Client 安装器。
- [install.ps1](install.ps1)：Windows Client 安装器。
