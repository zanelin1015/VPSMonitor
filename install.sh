#!/usr/bin/env bash
set -Eeuo pipefail

red='\033[0;31m'
green='\033[0;32m'
yellow='\033[0;33m'
blue='\033[0;34m'
plain='\033[0m'

repo="${VPSMONITOR_REPO:-zanelin1015/VPSMonitor}"
version="${VPSMONITOR_VERSION:-latest}"
prefix="${VPSMONITOR_PREFIX:-/opt/vpsmonitor}"
package_prefix="${VPSMONITOR_PACKAGE_PREFIX:-VPSMonitor}"
tmp_dir=""

cleanup() {
  if [[ -n "$tmp_dir" && -d "$tmp_dir" ]]; then
    rm -rf "$tmp_dir"
  fi
}
trap cleanup EXIT

info() { echo -e "${blue}$*${plain}"; }
ok() { echo -e "${green}$*${plain}"; }
warn() { echo -e "${yellow}$*${plain}"; }
die() { echo -e "${red}$*${plain}" >&2; exit 1; }

lower() {
  printf '%s' "$1" | tr '[:upper:]' '[:lower:]'
}

is_truthy() {
  case "$(lower "${1:-}")" in
    y | yes | true | 1 | on) return 0 ;;
    *) return 1 ;;
  esac
}

assume_yes() {
  is_truthy "${VPSMONITOR_ASSUME_YES:-${VPSMONITOR_NON_INTERACTIVE:-}}"
}

require_root() {
  [[ "${EUID:-$(id -u)}" -eq 0 ]] || die "Please run this script as root."
}

service_manager() {
  if command -v systemctl >/dev/null 2>&1; then
    echo "systemd"
    return
  fi
  if command -v rc-service >/dev/null 2>&1 && command -v rc-update >/dev/null 2>&1; then
    echo "openrc"
    return
  fi
  echo ""
}

require_service_manager() {
  [[ -n "$(service_manager)" ]] || die "systemd or OpenRC is required for service installation."
}

detect_arch() {
  case "$(uname -m)" in
    x86_64 | amd64) echo "amd64" ;;
    aarch64 | arm64) echo "arm64" ;;
    armv7l | armv7*) echo "armv7" ;;
    *) die "Unsupported CPU architecture: $(uname -m)" ;;
  esac
}

download_file() {
  local url="$1"
  local dst="$2"
  if command -v curl >/dev/null 2>&1; then
    local curl_args=(
      -fL
      --http1.1
      --retry "${VPSMONITOR_DOWNLOAD_RETRIES:-2}"
      --retry-delay 2
      --connect-timeout "${VPSMONITOR_CONNECT_TIMEOUT_SECONDS:-15}"
      --max-time "${VPSMONITOR_DOWNLOAD_TIMEOUT_SECONDS:-300}"
      --speed-limit "${VPSMONITOR_DOWNLOAD_MIN_BYTES_PER_SECOND:-1024}"
      --speed-time "${VPSMONITOR_DOWNLOAD_LOW_SPEED_SECONDS:-30}"
    )
    if curl --help all 2>/dev/null | grep -q -- '--retry-all-errors'; then
      curl_args+=(--retry-all-errors)
    fi
    curl "${curl_args[@]}" "$url" -o "$dst"
  elif command -v wget >/dev/null 2>&1; then
    wget --tries="${VPSMONITOR_DOWNLOAD_RETRIES:-2}" \
      --timeout="${VPSMONITOR_CONNECT_TIMEOUT_SECONDS:-15}" \
      -O "$dst" "$url"
  else
    die "curl or wget is required to download packages."
  fi
}

available_kb() {
  df -Pk "$1" 2>/dev/null | awk 'NR == 2 { print $4 }'
}

make_temp_dir() {
  local min_kb="${VPSMONITOR_MIN_TMP_KB:-65536}"
  local candidates=()
  [[ -n "${VPSMONITOR_TMP_DIR:-}" ]] && candidates+=("$VPSMONITOR_TMP_DIR")
  [[ -n "${TMPDIR:-}" ]] && candidates+=("$TMPDIR")
  candidates+=("/var/tmp" "/tmp" "${prefix%/}/.install-tmp")

  local base available
  for base in "${candidates[@]}"; do
    [[ -n "$base" ]] || continue
    if ! mkdir -p "$base" 2>/dev/null; then
      continue
    fi
    [[ -w "$base" ]] || continue
    available="$(available_kb "$base")"
    if [[ -n "$available" && "$available" =~ ^[0-9]+$ && "$available" -lt "$min_kb" ]]; then
      warn "Skipping temporary directory $base: only ${available}KB available, need at least ${min_kb}KB." >&2
      continue
    fi
    if tmp_dir="$(TMPDIR="$base" mktemp -d 2>/dev/null)"; then
      echo "$tmp_dir"
      return 0
    fi
  done

  die "No writable temporary directory with enough free space. Free /tmp space or set VPSMONITOR_TMP_DIR=/path/with/space and retry."
}

random_token() {
  local length="${1:-32}"
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex "$length" | cut -c 1-"$length"
  else
    dd if=/dev/urandom bs="$length" count=1 2>/dev/null | od -An -tx1 | tr -d ' \n' | cut -c 1-"$length"
  fi
}

json_escape() {
  printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g; s/\r//g; s/\t/\\t/g'
}

json_bool() {
  case "$(lower "$1")" in
    y | yes | true | 1) echo "true" ;;
    *) echo "false" ;;
  esac
}

sanitize_agent_id() {
  local value
  value="$(lower "$1" | sed 's/[^a-z0-9._-]/-/g; s/-\{2,\}/-/g; s/^-//; s/-$//')"
  if [[ -z "$value" ]]; then
    value="bridge-client"
  fi
  printf '%s' "$value" | cut -c 1-80 | sed 's/-$//'
}

default_client_agent_id() {
  local host suffix
  host="$(hostname 2>/dev/null || echo bridge-client)"
  host="$(sanitize_agent_id "$host")"
  suffix="$(random_token 8)"
  echo "${host}-${suffix}"
}

read_json_string_field() {
  local file="$1"
  local key="$2"
  [[ -f "$file" ]] || return 0
  sed -nE 's/^[[:space:]]*"'"$key"'"[[:space:]]*:[[:space:]]*"([^"]*)".*/\1/p' "$file" | head -n 1
}

prompt_default() {
  local __var="$1"
  local label="$2"
  local default="$3"
  local value=""
  if assume_yes; then
    printf -v "$__var" '%s' "$default"
    return
  fi
  read -r -p "$label [$default]: " value || true
  printf -v "$__var" '%s' "${value:-$default}"
}

prompt_required() {
  local __var="$1"
  local label="$2"
  local value=""
  while true; do
    read -r -p "$label: " value || true
    if [[ -n "$value" ]]; then
      printf -v "$__var" '%s' "$value"
      return
    fi
    warn "This value is required."
  done
}

prompt_secret_or_random() {
  local __var="$1"
  local label="$2"
  local generated
  generated="$(random_token 20)"
  local value=""
  while true; do
    read -r -s -p "$label (leave empty to generate): " value || true
    echo
    value="${value:-$generated}"
    if [[ "${#value}" -ge 8 ]]; then
      printf -v "$__var" '%s' "$value"
      return
    fi
    warn "Password must be at least 8 characters."
  done
}

confirm_default_no() {
  local label="$1"
  local value=""
  if assume_yes; then
    return 1
  fi
  read -r -p "$label [y/N]: " value || true
  value="$(lower "$value")"
  [[ "$value" == "y" || "$value" == "yes" ]]
}

confirm_default_yes() {
  local label="$1"
  local value=""
  if assume_yes; then
    return 0
  fi
  read -r -p "$label [Y/n]: " value || true
  value="$(lower "$value")"
  [[ -z "$value" || "$value" == "y" || "$value" == "yes" ]]
}

normalize_listen_addr() {
  local value="$1"
  if [[ "$value" =~ ^[0-9]+$ ]]; then
    echo ":$value"
  else
    echo "$value"
  fi
}

absolute_path() {
  local value="$1"
  [[ -n "$value" ]] || die "Path cannot be empty."
  if [[ "$value" == "/" ]]; then
    echo "/"
    return
  fi
  value="${value%/}"
  if [[ "$value" == /* ]]; then
    echo "$value"
  else
    echo "$(pwd -P)/$value"
  fi
}

listen_port() {
  local value="$1"
  if [[ "$value" =~ ^:([0-9]+)$ ]]; then
    echo "${BASH_REMATCH[1]}"
  elif [[ "$value" =~ ^[0-9]+$ ]]; then
    echo "$value"
  elif [[ "$value" =~ :([0-9]+)$ ]]; then
    echo "${BASH_REMATCH[1]}"
  fi
}

package_urls() {
  local component="$1"
  local arch="$2"
  local upper
  upper="$(printf '%s' "$component" | tr '[:lower:]' '[:upper:]')"
  local specific_var="VPSMONITOR_${upper}_PACKAGE_URL"
  local specific_url="${!specific_var:-}"
  if [[ -n "$specific_url" ]]; then
    echo "$specific_url"
    return
  fi
  if [[ -n "${VPSMONITOR_PACKAGE_URL:-}" ]]; then
    echo "$VPSMONITOR_PACKAGE_URL"
    return
  fi
  local package_name="${package_prefix}-${component}-linux-${arch}.tar.gz"
  if [[ -n "${VPSMONITOR_BASE_URL:-}" ]]; then
    echo "${VPSMONITOR_BASE_URL%/}/${package_name}"
  elif [[ "$version" == "latest" ]]; then
    echo "https://github.com/${repo}/releases/latest/download/${package_name}"
    echo "https://raw.githubusercontent.com/${repo}/main/dist/${package_name}"
  else
    echo "https://github.com/${repo}/releases/download/${version}/${package_name}"
    echo "https://raw.githubusercontent.com/${repo}/${version}/dist/${package_name}"
  fi
}

use_local_bundle() {
  is_truthy "${VPSMONITOR_USE_LOCAL_BUNDLE:-}"
}

script_dir() {
  local src="${BASH_SOURCE[0]}"
  if [[ -e "$src" ]]; then
    cd "$(dirname "$src")" >/dev/null 2>&1 && pwd
  else
    pwd
  fi
}

local_bundle_dir() {
  local component="$1"
  local dir
  dir="$(script_dir)"
  if [[ -x "$dir/bridge-$component" ]]; then
    echo "$dir"
    return
  fi
  return 1
}

fetch_bundle() {
  local component="$1"
  local arch="$2"
  local local_dir
  if use_local_bundle && local_dir="$(local_bundle_dir "$component")"; then
    info "Using local bridge-$component bundle:"
    echo "  $local_dir" >&2
    echo "$local_dir"
    return
  fi

  tmp_dir="${tmp_dir:-$(make_temp_dir)}"
  local package_path="$tmp_dir/${package_prefix}-${component}.tar.gz"
  local url downloaded="false"
  while IFS= read -r url; do
    [[ -n "$url" ]] || continue
    info "Downloading bridge-$component package:" >&2
    echo "  $url" >&2
    rm -f "$package_path"
    if download_file "$url" "$package_path"; then
      downloaded="true"
      break
    fi
    warn "Download source failed, trying the next source." >&2
  done < <(package_urls "$component" "$arch")
  if [[ "$downloaded" != "true" ]]; then
    local upper
    upper="$(printf '%s' "$component" | tr '[:lower:]' '[:upper:]')"
    die "Download failed. Set VPSMONITOR_${upper}_PACKAGE_URL or VPSMONITOR_BASE_URL and retry."
  fi
  tar -xzf "$package_path" -C "$tmp_dir"
  local binary
  binary="$(find "$tmp_dir" -type f -name "bridge-$component" | head -n 1 || true)"
  [[ -n "$binary" ]] || die "bridge-$component binary was not found in package."
  dirname "$binary"
}

write_server_config() {
  local config_path="$1"
  local install_dir="$2"
  local listen_addr="$3"
  local data_dir="$4"
  local registration_token="$5"
  local admin_username="$6"
  local admin_password="$7"
  local retention_days="$8"
  local retention_count="$9"

  mkdir -p "$(dirname "$config_path")" "$data_dir"
  (
    umask 077
    cat >"$config_path" <<EOF
{
  "listen_addr": "$(json_escape "$listen_addr")",
  "data_dir": "$(json_escape "$data_dir")",
  "database_path": "$(json_escape "$data_dir/bridge.db")",
  "credential_key_path": "$(json_escape "$data_dir/credential.key")",
  "registration_token": "$(json_escape "$registration_token")",
  "admin_username": "$(json_escape "$admin_username")",
  "admin_password": "$(json_escape "$admin_password")",
  "snapshot_retention_days": $retention_days,
  "snapshot_retention_count": $retention_count,
  "agents": []
}
EOF
  )
  chmod 600 "$config_path"
  mkdir -p "$install_dir"
}

write_client_config() {
  local config_path="$1"
  local server_url="$2"
  local agent_id="$3"
  local registration_token="$4"
  local skip_tls_verify="$5"
  local poll_interval="$6"
  local request_timeout="$7"

  mkdir -p "$(dirname "$config_path")"
  (
    umask 077
    cat >"$config_path" <<EOF
{
  "agent_id": "$(json_escape "$agent_id")",
  "registration_token": "$(json_escape "$registration_token")",
  "agent_token": "",
  "server_url": "$(json_escape "$server_url")",
  "server_skip_tls_verify": $(json_bool "$skip_tls_verify"),
  "poll_interval": "$(json_escape "$poll_interval")",
  "request_timeout_seconds": $request_timeout
}
EOF
  )
  chmod 600 "$config_path"
}

install_service() {
  local service_name="$1"
  local description="$2"
  local install_dir="$3"
  local binary_name="$4"
  local config_path="$5"
  local manager
  manager="$(service_manager)"
  case "$manager" in
    systemd)
      install_systemd_service "$service_name" "$description" "$install_dir" "$binary_name" "$config_path"
      ;;
    openrc)
      install_openrc_service "$service_name" "$description" "$install_dir" "$binary_name" "$config_path"
      ;;
    *)
      die "systemd or OpenRC is required for service installation."
      ;;
  esac
}

install_systemd_service() {
  local service_name="$1"
  local description="$2"
  local install_dir="$3"
  local binary_name="$4"
  local config_path="$5"
  local service_file="/etc/systemd/system/${service_name}.service"

  cat >"$service_file" <<EOF
[Unit]
Description=$description
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=$install_dir
ExecStart=$install_dir/$binary_name -config $config_path
Restart=always
RestartSec=3
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
EOF

  systemctl daemon-reload
  systemctl unmask "$service_name" >/dev/null 2>&1 || true
  systemctl enable --now "$service_name" >/dev/null
  systemctl restart "$service_name"
  if ! systemctl is-enabled "$service_name" >/dev/null 2>&1; then
    die "failed to enable $service_name for startup after boot."
  fi
  if ! systemctl is-active "$service_name" >/dev/null 2>&1; then
    die "failed to start $service_name after installation."
  fi
}

install_openrc_service() {
  local service_name="$1"
  local description="$2"
  local install_dir="$3"
  local binary_name="$4"
  local config_path="$5"
  local service_file="/etc/init.d/${service_name}"
  mkdir -p /run /var/log

  cat >"$service_file" <<EOF
#!/sbin/openrc-run
name="$description"
description="$description"
directory="$install_dir"
command="$install_dir/$binary_name"
command_args="-config $config_path"
command_background=true
pidfile="/run/\${RC_SVCNAME}.pid"
output_log="/var/log/\${RC_SVCNAME}.log"
error_log="/var/log/\${RC_SVCNAME}.log"
start_stop_daemon_args="--make-pidfile"

depend() {
  need net
  after firewall
}
EOF

  chmod +x "$service_file"
  rc-update add "$service_name" default >/dev/null
  if rc-service "$service_name" status >/dev/null 2>&1; then
    rc-service "$service_name" restart
  else
    rc-service "$service_name" start
  fi
  if ! rc-update show default | grep -Eq "(^|[[:space:]])${service_name}([[:space:]]|$)"; then
    die "failed to enable $service_name in OpenRC default runlevel."
  fi
}

service_status_hint() {
  local service_name="$1"
  case "$(service_manager)" in
    systemd) echo "systemctl status $service_name" ;;
    openrc) echo "rc-service $service_name status" ;;
    *) echo "check service status manually" ;;
  esac
}

service_logs_hint() {
  local service_name="$1"
  case "$(service_manager)" in
    systemd) echo "journalctl -u $service_name -f" ;;
    openrc) echo "tail -f /var/log/$service_name.log" ;;
    *) echo "check service logs manually" ;;
  esac
}

install_server() {
  local arch="$1"
  local source_dir
  source_dir="$(fetch_bundle server "$arch")"
  local install_dir="${VPSMONITOR_SERVER_DIR:-$prefix/server}"
  prompt_default install_dir "Install directory for bridge-server" "$install_dir"
  install_dir="$(absolute_path "$install_dir")"
  local config_path="$install_dir/config/server.json"
  local data_dir="$install_dir/data"
  local listen_addr=":8090"
  local registration_token
  registration_token="$(random_token 32)"
  local admin_username="admin"
  local admin_password=""
  local retention_days="30"
  local retention_count="5000"

  if [[ -f "$config_path" ]]; then
    warn "Existing server config found: $config_path"
    if ! confirm_default_no "Overwrite and reconfigure it"; then
      info "Keeping existing server config."
    else
      prompt_default listen_addr "Server listen address, e.g. :8090 or 127.0.0.1:8090" "$listen_addr"
      listen_addr="$(normalize_listen_addr "$listen_addr")"
      prompt_default data_dir "Server data directory" "$data_dir"
      data_dir="$(absolute_path "$data_dir")"
      prompt_default registration_token "Client registration token" "$registration_token"
      prompt_default admin_username "Initial admin username" "$admin_username"
      prompt_secret_or_random admin_password "Initial admin password"
      prompt_default retention_days "Snapshot retention days" "$retention_days"
      prompt_default retention_count "Snapshot retention count per agent" "$retention_count"
      write_server_config "$config_path" "$install_dir" "$listen_addr" "$data_dir" "$registration_token" "$admin_username" "$admin_password" "$retention_days" "$retention_count"
    fi
  else
    info "Server config"
    prompt_default listen_addr "Server listen address, e.g. :8090 or 127.0.0.1:8090" "$listen_addr"
    listen_addr="$(normalize_listen_addr "$listen_addr")"
    prompt_default data_dir "Server data directory" "$data_dir"
    data_dir="$(absolute_path "$data_dir")"
    prompt_default registration_token "Client registration token" "$registration_token"
    prompt_default admin_username "Initial admin username" "$admin_username"
    prompt_secret_or_random admin_password "Initial admin password"
    prompt_default retention_days "Snapshot retention days" "$retention_days"
    prompt_default retention_count "Snapshot retention count per agent" "$retention_count"
    write_server_config "$config_path" "$install_dir" "$listen_addr" "$data_dir" "$registration_token" "$admin_username" "$admin_password" "$retention_days" "$retention_count"
  fi

  mkdir -p "$install_dir"
  install -m 0755 "$source_dir/bridge-server" "$install_dir/bridge-server"
  [[ -f "$source_dir/README.md" ]] && install -m 0644 "$source_dir/README.md" "$install_dir/README.md"

  local service_name="${VPSMONITOR_SERVER_SERVICE:-vpsmonitor-server}"
  prompt_default service_name "Service name" "$service_name"
  install_service "$service_name" "VPSMonitor Bridge Server" "$install_dir" "bridge-server" "$config_path"

  ok "bridge-server installed."
  echo "  Service: $service_name"
  echo "  Config:  $config_path"
  echo "  Status:  $(service_status_hint "$service_name")"
  local port
  port="$(listen_port "$listen_addr")"
  if [[ -n "$port" ]]; then
    echo "  Console: http://SERVER_IP:$port/"
  fi
  if [[ -n "$admin_password" ]]; then
    echo
    echo "Please save these values:"
    echo "  Admin username: $admin_username"
    echo "  Admin password: $admin_password"
    echo "  Registration token: $registration_token"
  fi
}

install_client() {
  local arch="$1"
  local source_dir
  source_dir="$(fetch_bundle client "$arch")"
  local install_dir="${VPSMONITOR_CLIENT_DIR:-$prefix/client}"
  prompt_default install_dir "Install directory for bridge-client" "$install_dir"
  install_dir="$(absolute_path "$install_dir")"
  local config_path="$install_dir/config/client.json"
  local server_url="${VPSMONITOR_SERVER_URL:-http://SERVER_IP:8090}"
  local agent_id="${VPSMONITOR_AGENT_ID:-}"
  local existing_agent_id=""
  local registration_token="${VPSMONITOR_REGISTRATION_TOKEN:-}"
  local skip_tls_verify="${VPSMONITOR_SERVER_SKIP_TLS_VERIFY:-n}"
  local poll_interval="${VPSMONITOR_POLL_INTERVAL:-30s}"
  local request_timeout="${VPSMONITOR_REQUEST_TIMEOUT_SECONDS:-15}"
  existing_agent_id="$(read_json_string_field "$config_path" "agent_id")"
  if [[ -z "$agent_id" && -n "$existing_agent_id" ]]; then
    agent_id="$existing_agent_id"
  fi
  if [[ -z "$agent_id" ]]; then
    agent_id="$(default_client_agent_id)"
  else
    agent_id="$(sanitize_agent_id "$agent_id")"
  fi

  if [[ -f "$config_path" ]]; then
    warn "Existing client config found: $config_path"
    if ! is_truthy "${VPSMONITOR_FORCE_CONFIG:-}" && ! confirm_default_no "Overwrite and reconfigure it"; then
      info "Keeping existing client config."
    else
      prompt_default server_url "Server URL" "$server_url"
      if [[ -z "$registration_token" ]]; then
        prompt_required registration_token "Client registration token"
      fi
      prompt_default agent_id "Client ID" "$agent_id"
      agent_id="$(sanitize_agent_id "$agent_id")"
      prompt_default skip_tls_verify "Skip server TLS verification? y/N" "$skip_tls_verify"
      prompt_default poll_interval "Poll interval" "$poll_interval"
      prompt_default request_timeout "Request timeout seconds" "$request_timeout"
      write_client_config "$config_path" "$server_url" "$agent_id" "$registration_token" "$skip_tls_verify" "$poll_interval" "$request_timeout"
    fi
  else
    info "Client config"
    prompt_default server_url "Server URL" "$server_url"
    if [[ -z "$registration_token" ]]; then
      if assume_yes; then
        die "VPSMONITOR_REGISTRATION_TOKEN is required for non-interactive client installation."
      fi
      prompt_required registration_token "Client registration token"
    fi
    prompt_default agent_id "Client ID" "$agent_id"
    agent_id="$(sanitize_agent_id "$agent_id")"
    prompt_default skip_tls_verify "Skip server TLS verification? y/N" "$skip_tls_verify"
    prompt_default poll_interval "Poll interval" "$poll_interval"
    prompt_default request_timeout "Request timeout seconds" "$request_timeout"
    write_client_config "$config_path" "$server_url" "$agent_id" "$registration_token" "$skip_tls_verify" "$poll_interval" "$request_timeout"
  fi

  mkdir -p "$install_dir"
  install -m 0755 "$source_dir/bridge-client" "$install_dir/bridge-client"
  [[ -f "$source_dir/README.md" ]] && install -m 0644 "$source_dir/README.md" "$install_dir/README.md"

  local service_name="${VPSMONITOR_CLIENT_SERVICE:-vpsmonitor-client}"
  prompt_default service_name "Service name" "$service_name"
  install_service "$service_name" "VPSMonitor Bridge Client" "$install_dir" "bridge-client" "$config_path"

  ok "bridge-client installed."
  echo "  Service: $service_name"
  echo "  Config:  $config_path"
  local final_agent_id
  final_agent_id="$(read_json_string_field "$config_path" "agent_id")"
  [[ -n "$final_agent_id" ]] && echo "  Client ID: $final_agent_id"
  echo "  Status:  $(service_status_hint "$service_name")"
  echo "  Logs:    $(service_logs_hint "$service_name")"
}

usage() {
  cat <<EOF
Usage:
  bash install.sh [server|client|both]

Environment overrides:
  VPSMONITOR_REPO=zanelin1015/VPSMonitor
  VPSMONITOR_VERSION=latest
  VPSMONITOR_PACKAGE_PREFIX=VPSMonitor
  VPSMONITOR_TMP_DIR=/var/tmp
  VPSMONITOR_USE_LOCAL_BUNDLE=true
  VPSMONITOR_DOWNLOAD_RETRIES=2
  VPSMONITOR_CONNECT_TIMEOUT_SECONDS=15
  VPSMONITOR_DOWNLOAD_TIMEOUT_SECONDS=300
  VPSMONITOR_DOWNLOAD_LOW_SPEED_SECONDS=30
  VPSMONITOR_DOWNLOAD_MIN_BYTES_PER_SECOND=1024
  VPSMONITOR_BASE_URL=https://example.com/downloads
  VPSMONITOR_SERVER_PACKAGE_URL=https://example.com/VPSMonitor-server-linux-amd64.tar.gz
  VPSMONITOR_CLIENT_PACKAGE_URL=https://example.com/VPSMonitor-client-linux-amd64.tar.gz
  VPSMONITOR_SERVER_URL=https://panel.example.com
  VPSMONITOR_REGISTRATION_TOKEN=token-from-server
  VPSMONITOR_SERVER_SKIP_TLS_VERIFY=false
  VPSMONITOR_POLL_INTERVAL=30s
  VPSMONITOR_REQUEST_TIMEOUT_SECONDS=15
  VPSMONITOR_ASSUME_YES=true
  VPSMONITOR_FORCE_CONFIG=true
EOF
}

main() {
  local action="${1:-server}"
  case "$action" in
    -h | --help)
      usage
      exit 0
      ;;
    server | client | both) ;;
    *)
      warn "Unknown install target: $action"
      usage
      exit 1
      ;;
  esac

  require_root
  require_service_manager
  command -v tar >/dev/null 2>&1 || die "tar is required."

  local arch
  arch="$(detect_arch)"
  info "VPSMonitor installer"
  echo "  Target: $action"
  echo "  Arch:   linux/$arch"
  echo "  Repo:   $repo"
  echo "  Ver:    $version"
  echo "  Package prefix: $package_prefix"
  if use_local_bundle; then
    echo "  Bundle source: local"
  else
    echo "  Bundle source: release download"
  fi
  echo "  Service manager: $(service_manager)"
  echo

  case "$action" in
    server)
      install_server "$arch"
      ;;
    client)
      install_client "$arch"
      ;;
    both)
      install_server "$arch"
      echo
      install_client "$arch"
      ;;
  esac
}

main "$@"
