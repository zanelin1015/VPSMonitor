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

require_systemd() {
  command -v systemctl >/dev/null 2>&1 || die "systemctl is required for service installation."
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
    curl -fL --retry 3 --connect-timeout 15 "$url" -o "$dst"
  elif command -v wget >/dev/null 2>&1; then
    wget -O "$dst" "$url"
  else
    die "curl or wget is required to download packages."
  fi
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

package_url() {
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
  else
    echo "https://github.com/${repo}/releases/download/${version}/${package_name}"
  fi
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
  if local_dir="$(local_bundle_dir "$component")"; then
    echo "$local_dir"
    return
  fi

  tmp_dir="${tmp_dir:-$(mktemp -d)}"
  local package_path="$tmp_dir/${package_prefix}-${component}.tar.gz"
  local url
  url="$(package_url "$component" "$arch")"
  info "Downloading bridge-$component package:" >&2
  echo "  $url" >&2
  if ! download_file "$url" "$package_path"; then
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
  local registration_token="$3"
  local skip_tls_verify="$4"
  local poll_interval="$5"
  local request_timeout="$6"

  mkdir -p "$(dirname "$config_path")"
  (
    umask 077
    cat >"$config_path" <<EOF
{
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
  systemctl enable "$service_name" >/dev/null
  systemctl restart "$service_name"
}

install_server() {
  local arch="$1"
  local source_dir
  source_dir="$(fetch_bundle server "$arch")"
  local install_dir="${VPSMONITOR_SERVER_DIR:-$prefix/server}"
  prompt_default install_dir "Install directory for bridge-server" "$install_dir"
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
  prompt_default service_name "Systemd service name" "$service_name"
  install_service "$service_name" "VPSMonitor Bridge Server" "$install_dir" "bridge-server" "$config_path"

  ok "bridge-server installed."
  echo "  Service: $service_name"
  echo "  Config:  $config_path"
  echo "  Status:  systemctl status $service_name"
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
  local config_path="$install_dir/config/client.json"
  local server_url="${VPSMONITOR_SERVER_URL:-http://SERVER_IP:8090}"
  local registration_token="${VPSMONITOR_REGISTRATION_TOKEN:-}"
  local skip_tls_verify="${VPSMONITOR_SERVER_SKIP_TLS_VERIFY:-n}"
  local poll_interval="${VPSMONITOR_POLL_INTERVAL:-30s}"
  local request_timeout="${VPSMONITOR_REQUEST_TIMEOUT_SECONDS:-15}"

  if [[ -f "$config_path" ]]; then
    warn "Existing client config found: $config_path"
    if ! is_truthy "${VPSMONITOR_FORCE_CONFIG:-}" && ! confirm_default_no "Overwrite and reconfigure it"; then
      info "Keeping existing client config."
    else
      prompt_default server_url "Server URL" "$server_url"
      if [[ -z "$registration_token" ]]; then
        prompt_required registration_token "Client registration token"
      fi
      prompt_default skip_tls_verify "Skip server TLS verification? y/N" "$skip_tls_verify"
      prompt_default poll_interval "Poll interval" "$poll_interval"
      prompt_default request_timeout "Request timeout seconds" "$request_timeout"
      write_client_config "$config_path" "$server_url" "$registration_token" "$skip_tls_verify" "$poll_interval" "$request_timeout"
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
    prompt_default skip_tls_verify "Skip server TLS verification? y/N" "$skip_tls_verify"
    prompt_default poll_interval "Poll interval" "$poll_interval"
    prompt_default request_timeout "Request timeout seconds" "$request_timeout"
    write_client_config "$config_path" "$server_url" "$registration_token" "$skip_tls_verify" "$poll_interval" "$request_timeout"
  fi

  mkdir -p "$install_dir"
  install -m 0755 "$source_dir/bridge-client" "$install_dir/bridge-client"
  [[ -f "$source_dir/README.md" ]] && install -m 0644 "$source_dir/README.md" "$install_dir/README.md"

  local service_name="${VPSMONITOR_CLIENT_SERVICE:-vpsmonitor-client}"
  prompt_default service_name "Systemd service name" "$service_name"
  install_service "$service_name" "VPSMonitor Bridge Client" "$install_dir" "bridge-client" "$config_path"

  ok "bridge-client installed."
  echo "  Service: $service_name"
  echo "  Config:  $config_path"
  echo "  Status:  systemctl status $service_name"
  echo "  Logs:    journalctl -u $service_name -f"
}

usage() {
  cat <<EOF
Usage:
  bash install.sh [server|client|both]

Environment overrides:
  VPSMONITOR_REPO=zanelin1015/VPSMonitor
  VPSMONITOR_VERSION=latest
  VPSMONITOR_PACKAGE_PREFIX=VPSMonitor
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
  require_systemd
  command -v tar >/dev/null 2>&1 || die "tar is required."

  local arch
  arch="$(detect_arch)"
  info "VPSMonitor installer"
  echo "  Target: $action"
  echo "  Arch:   linux/$arch"
  echo "  Repo:   $repo"
  echo "  Ver:    $version"
  echo "  Package prefix: $package_prefix"
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
