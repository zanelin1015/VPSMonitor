# VPSMonitor

VPSMonitor 是一套围绕 `x-ui / 3x-ui` 的 VPS 集中管理系统。它不替代每台机器上的 x-ui 面板，而是在其上提供统一的 Client 注册、监控、配置编排、链路拓扑、客户订阅和运维入口。

项目由两部分组成：

- `bridge-server`：中心服务、SQLite 数据库和内嵌 Web 管理台。
- `bridge-client`：部署在每台 VPS 或 iStoreOS/OpenWrt 上，采集本机状态、连接本机 x-ui，并执行 Server 下发的任务。

当前版本的默认数据文件是 `./data/bridge.db`，生产环境建议把 `data` 放在独立且可备份的持久化目录中。

## 能做什么

管理端可以完成：

- Client 自动注册、在线状态、CPU/内存/磁盘、网速和流量监控。
- x-ui 节点、客户端、出站和路由规则查看与下发；路由规则支持原位编辑、常用条件与高级 JSON，显示有效、部分有效、无效或停用状态及原因；路由与出站支持单条和多选删除，并保护系统 API 路由与默认出口。
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

Server 会保存 Client 基本信息、最近快照、历史快照、x-ui 托管配置、操作记录、访问日志和客户公告已读记录。x-ui API Token 和兼容旧配置的密码使用 `credential_key_path` 指定的本地密钥加密后写入 SQLite，数据库和密钥文件需要一起备份。

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
sudo env VPSMONITOR_VERSION=v0.3.37 \
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

Customer 登录使用两个独立的 HttpOnly Cookie：登录凭证有效期 2 小时，refresh 凭证有效期 30 天。前端收到 Customer 接口的 401 时，会通过 `POST /api/v1/customer/refresh` 获取新的登录凭证，并重试原请求一次；并发请求共用同一次续期。refresh 仅用于续期，不能直接访问业务接口；其 30 天期限固定从登录时计算，续期不延长期限，临近期限签发的登录凭证也不会超过该期限。退出登录会撤销当前设备的两种凭证；禁用账号或管理员重置密码会撤销该账号的所有凭证。客户自行改密保留当前设备登录，其他设备需要重新登录。数据库只保存凭证哈希。升级前已有登录凭证保留原有效期，再次登录后启用双凭证；订阅 Token 不受此机制影响。

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

Client 的 x-ui 访问参数由 Server 集中管理。管理员在 Client 详情页的“X-ui 配置”中维护 Base URL 和 API Token；Token 模式无需填写面板用户名、密码或二步验证码，这些登录字段已从表单移除。Token 被拒绝时不会改用旧账号登录；未配置 Token 的已有旧式配置继续兼容。Client 会在下一次轮询时拉取配置。

本地 x-ui 数据库路径位于“高级设置”，用于 API 不可用时的本地兜底读取。常规安装留空即可自动探测；Docker 或非标准安装可填写 Client 本机可读取的数据库路径。

X-ui 配置中的开关紧邻说明，窄屏自动换行。“日志保留天数”和“Xray access.log 路径”仅在开启“采集访问日志”后显示；关闭时保留已填写的值，再次开启后恢复显示。

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

只预览本地前端、读取线上 API 时，无需启动本地 Go Server。在 `web` 目录执行：

```bash
VPSMONITOR_API_TARGET='https://your-server-hostname/zanelin' npm run dev -- --host 127.0.0.1 --port 5173 --strictPort
```

打开 `http://localhost:5173/`，使用线上账号登录。代理目标填 API 的部署前缀，不要带 `/monitor` 或页面查询参数；根路径部署则不带 `/zanelin`。开发代理会映射登录 Cookie 路径和实时连接 Origin，但保留 Cookie 安全属性及上游 TLS 校验。服务仅绑定回环地址，不要将此代理暴露到公网。页面中的保存、删除、重启等操作仍会修改线上数据。

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

在线升级弹窗中的「Client 升级日志」保存每次批量下发的逐台结果，包括北京时间、Client 名称/ID、原版本和目标版本、下发或跳过结果及原因。可按名称、版本、原因搜索并筛选跳过记录，默认显示最近 200 条；重启或删除 Client 不会删除历史日志。系统/架构未知、安装包或校验值缺失、已是最新、旧 Client 不支持安全升级及已有升级任务均会明确记录原因。可升级台数使用与实际下发相同的版本、平台和安全升级能力检查。

日志将「待领取」「执行中」「已启动，待确认」「执行失败」「结果未确认」与「升级成功」区分显示。Client 成功启动升级程序不等于安装成功，只有完成回执之后重新上报目标版本才确认升级成功。弹窗打开时每 10 秒刷新；旧版本没有保存过的历史跳过记录无法补录。

路由规则删除要求 Server 和目标 Client 均升级到 0.3.31 或更高版本。删除前会校验完整规则列表及所选规则，规则发生变化时拒绝删除并要求刷新；多选按索引倒序逐条移除，保存后重载 Xray。系统 API 路由不可删除。如果提示规则已保存但重载失败，请手动重启 x-ui / Xray，不要重复提交删除。

路由列表中的编辑功能要求 Server 和目标 Client 均升级到 v0.3.50 或更高版本。编辑器读取面板保存的完整单条规则，常用条件表单和高级 JSON 共用同一份草稿，取消不会下发；修改后校验完整规则列表与目标规则指纹，原位替换且保留顺序、其他规则和未编辑字段。保存后回读核验，再重载 Xray；仅全部完成才显示成功。系统 API 路由不可修改。若配置已保存但核验或重载失败，请刷新核对或手动重启，不要重复保存。

路由有效状态按采集到的配置判断：全局条件和系统 API 路由视为有效，面板已停用的规则显示停用；其他规则核对入站、客户端（含 `regexp:` 名称匹配）、出站和均衡器引用。入站和客户端条件须同时匹配，部分对象不存在时显示部分有效。在线状态、流量和目标网络连通性不参与这项配置检查。

出站与路由规则支持列表、卡片切换，初次打开默认列表，当前页面内分别保留查看方式及勾选项。出站的关联路由明细展示直接引用、均衡器候选与回退关联，包含停用和失效规则，可定位到对应规则。没有显式引用不代表默认出口没有流量；旧 Client 未上报保存模板时，关联仅按运行时规则展示。

出站批量删除要求目标 VPSMonitor Client v0.3.49 或更高版本，升级后需刷新采集。3x-ui 的 `/panel/api/xray/update` 接口接收完整配置，所选出站一次移除、一次保存，并回读核验后重载 Xray；不循环提交单条删除。删除前校验完整配置及每个出站，保护默认出口、系统 API 出站及仍被路由、均衡器、探测配置或代理链引用的出站。运行时或订阅生成的出站在来源面板维护。若保存成功但核验或重载失败，操作记录显示失败及已完成阶段，需刷新核对或手动重启，避免重复删除。

## NAT / Docker 连接与端口

在「Client 详情 → 连接与端口」设置 x-ui 的部署方式（直接安装 / Docker）、Docker 网络（Host 端口直通 / Bridge 端口映射）及公网接入方式（直接访问 / NAT）。公网入口填写供应商提供的域名或 IP，不含 URL 协议、路径或端口；NAT 的出口 IP 不一定是可连接的入口地址。

端口映射绑定实际 x-ui 入站、Realm 或 HAProxy 规则，同一入站的所有客户端共用。Docker Bridge 的「服务器端口」对应容器映射到宿主机的端口，NAT 的「公网端口」对应最外层可访问端口。例如公网 `333 → 服务器 30001 → 容器 20001`，节点导出使用 `333`；Docker Host 直接访问时沿用监听端口。TCP / UDP 需与实际映射一致，同一节点有多个入口时选择一个默认导出入口。

保存后，节点链接、二维码、客户订阅、新建出站及转发链路使用解析后的公网入口，内部监听端口及 UUID、TLS SNI、REALITY 参数保留原值。映射缺失、绑定过期、协议不匹配或默认入口不明确时显示原因并阻止导出，避免生成不可用的内部端口。旧拓扑映射保留原用途，只有选择「用于节点导出」并绑定节点后才参与导出。

此功能登记已有映射，不会创建 Docker 端口映射、修改防火墙或供应商 NAT 设置。保存后更新订阅；已经导入的静态节点需重新导入，已保存的出站需重新选择来源并保存。升级会保留原有 Client 配置与映射。

## Clash/Mihomo 与新版 REALITY

客户订阅根据实际承接 VLESS REALITY 入站的 Xray 核心版本生成握手参数。Xray 26.9.8 及更新版本输出 `reality-opts.support-x25519mlkem768: true`，并使用支持该密钥交换的 `client-fingerprint: chrome`；旧版本和核心版本未知时保留原订阅参数。普通 TLS 节点不受影响，Realm/HAProxy 转发链路使用后端 Xray 入站的版本，而不是端口转发机器的版本。未托管的第三方前置代理无法自动判断核心版本。

管理端和客户门户的单节点复制链接、二维码默认选择 Shadowrocket。对于 Xray 26.9.8 及更新版本的 VLESS REALITY 入站，系统自动生成带 `fp=chrome140` 的 Shadowrocket 链接，导入后无需手动修改 TLS 指纹。导入其他软件可在 URL 弹窗或客户授权链路页切换“其他客户端”，浏览器会记住选择；通用链接和 Clash/Mihomo 订阅仍使用各自的参数。旧版核心、未知核心以及普通 TLS 链接保持原样。已有设备需重新导入系统生成的新链接。

升级 Xray 后先刷新该 Client 的信息，再在 Clash Verge 中更新订阅并重新连接。Mihomo v1.19.32 支持此选项，但默认未开启；仅升级连接端核心不会补全旧订阅中缺少的握手参数。

## x-ui 客户端自动流量重置

客户端列表以只读摘要展示流量（`↑` / `↓`）、流量倍率、费用、开始日期及到期日期，启用状态可以通过开关直接切换。点击“编辑”可在同一弹窗修改启用状态、总流量上限、金额/币种/收费周期、流量倍数、开始及到期时间、自动重置规则，点击“保存全部”统一提交。未保存的修改仅保存在弹窗草稿中；手动指定的到期时间优先。“按开始日每月重置”取弹窗内正在编辑的开始日号，短月月底执行。

流量列新增「手动重置」，确认后仅对选定的客户端调用 x-ui 原生流量重置，并回读流量与配置核验。它保留启用状态、总流量上限、到期时间与自动重置规则，不恢复旧的定时任务。需要 Server 和目标 **VPSMonitor Client v0.3.51 或更高版本**；转发客户端会解析到实际出口 Client。请求结果不明确时不会自动重试，应先刷新核对流量。在线客户端清零后可能立即产生新流量。

统一接口为 `POST /api/v1/agents/{agent_id}/xui/client-config`，结果查询为 `GET /api/v1/agents/{agent_id}/xui/client-config/{request_id}`，沿用管理台 Cookie 鉴权，仅允许管理客户端配置的 Admin 使用。`request_id` 用于重复提交去重；只更新该客户端费用并在同一事务中创建远端任务，不会保存其他表单草稿。转发客户端由 Server 解析实际出口 Client，费用仍记录在入口 Client。远端修改通过 `update_client_config` 合并写入 x-ui，再逐字段回读核验；列表和弹窗显示待下发、执行中、已生效或部分失败，支持只重试未确认的字段。不同系统间不作跨库原子成功承诺，保存重置规则不会立即清零。

合并下发需要目标 **VPSMonitor Client v0.3.49 或更高版本**（不是 Xray / x-ui 版本）。旧 Client 或版本未上报时，会在保存前提示升级；只修改本系统费用不要求此版本。HTTP/SOCKS 节点账号仅修改本系统费用和时间记录，现有区域账号权限保持原范围。已知不支持自动重置的旧面板仍可修改其他支持的字段。

管理台「Client 详情 → 客户端 → 流量 → 自动重置 → 设置」可配置 x-ui 原生周期：从不、每小时、每日、每周、每月。选择每月时可设置 1–31 号，默认 1 号；不足该日期的月份按月底执行。按 x-ui 面板时区在对应边界执行，每周为周日 00:00，每月为指定日 00:00，与客户端到期日独立。

重置周期另提供“每月 N 号（按开始日）”快捷选项，取当前客户端“开始 / 到期”栏开始日期的日号，按北京时间读取日期。10 月 1 日或 10 月 10 日开始的季付客户端分别保存为每月 1 号或 10 号重置。31 号开始时始终保存日号 31，2 月按 28/29 号、4 月按 30 号执行，3 月仍按 31 号执行。没有可用的开始日期时此选项不可选；调整开始日期后需重新选择并保存。

此设置由 x-ui 执行，保存时不立即清零流量，也不修改总流量上限、到期时间和凭据。同一客户端关联多个入站时同步生效。系统不再执行独立的周期清零定时任务。

需升级 Server 和目标 VPSMonitor Client 到 0.3.46 或更高版本，并使用支持客户端重置周期的新版 x-ui。旧 Client 在下发前提示升级，旧面板不会收到无效的入站修改。保存后回读周期和日期，实际写入一致后才报告 `verified: true`。

也可调用现有异步操作接口（使用管理台会话鉴权），在操作记录中查看最终执行结果：

```http
POST /api/v1/agents/{agent_id}/xui/actions
Content-Type: application/json

{
  "kind": "update_client_traffic_reset",
  "payload": {
    "inbound_id": 7,
    "email": "alice",
    "traffic_reset": "monthly",
    "traffic_reset_day": 15
  }
}
```

`traffic_reset` 可取 `never`、`hourly`、`daily`、`weekly`、`monthly`；`traffic_reset_day` 为 1–31 的整数，省略时默认 1。

升级后旧的系统周期重置入口和 API 已移除，尚未下发的 `reset_client_traffic` 任务会失效，旧 Client 的执行前校验也会拒绝继续清零。已有策略和历史日志保留在数据库中，不再调度；旧策略不会自动迁移到 x-ui，请在上述入口设置需要的原生周期。

定时任务页面仅保留告警扫描和每日流量日报，Customer 页面不再显示由收费开始日推算的旧重置时间。收费周期与到期时间仍独立管理：编辑开始日期或收费周期时计算到期时间，手动填写的到期时间优先，读取配置或保存费用不会自动续期。客户端提供北京时间精确到秒的手动到期输入。

## 打包与测试

Linux/macOS：

```bash
chmod +x ./scripts/build.sh
./scripts/build.sh
```

指定版本：

```bash
VPSMONITOR_BUILD_VERSION=0.3.37 ./scripts/build.sh
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
$env:VPSMONITOR_BUILD_VERSION = "0.3.37"
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
npm run test:routing-editor
npm run test:outbound-references
npm run test:app-base
npm run test:app-recovery
npm run test:entry-connection
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
