#!/bin/sh
set -eu

repo="${VPSMONITOR_REPO:-zanelin1015/VPSMonitor}"
version="${VPSMONITOR_VERSION:-latest}"
package_prefix="${VPSMONITOR_PACKAGE_PREFIX:-VPSMonitor}"
prefix="${VPSMONITOR_PREFIX:-/opt/vpsmonitor}"
tmp_dir=""
bundle_dir=""

info() { printf '%s\n' "$*"; }
warn() { printf 'Warning: %s\n' "$*" >&2; }
die() { printf 'Error: %s\n' "$*" >&2; exit 1; }

cleanup() {
  if [ -n "$tmp_dir" ] && [ -d "$tmp_dir" ]; then
    rm -rf "$tmp_dir"
  fi
}
trap cleanup EXIT INT TERM

lower() {
  printf '%s' "$1" | tr '[:upper:]' '[:lower:]'
}

is_truthy() {
  case "$(lower "${1:-}")" in
    y|yes|true|1|on) return 0 ;;
    *) return 1 ;;
  esac
}

assume_yes() {
  is_truthy "${VPSMONITOR_ASSUME_YES:-${VPSMONITOR_NON_INTERACTIVE:-}}"
}

require_openwrt() {
  [ "$(id -u)" -eq 0 ] || die "Please run this script as root."
  [ -f /etc/openwrt_release ] || [ -x /sbin/procd ] || die "This installer is only for OpenWrt/iStoreOS with procd."
  [ -x /etc/rc.common ] || die "/etc/rc.common was not found."
  command -v tar >/dev/null 2>&1 || die "tar is required. Install it with: opkg update && opkg install tar"
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64) echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    armv7l|armv7*|armhf) echo "arm" ;;
    *) die "Unsupported CPU architecture: $(uname -m)" ;;
  esac
}

download_file() {
  url="$1"
  dst="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fL --connect-timeout "${VPSMONITOR_CONNECT_TIMEOUT_SECONDS:-15}" \
      --max-time "${VPSMONITOR_DOWNLOAD_TIMEOUT_SECONDS:-300}" "$url" -o "$dst"
    return
  fi
  if command -v uclient-fetch >/dev/null 2>&1; then
    uclient-fetch -T "${VPSMONITOR_DOWNLOAD_TIMEOUT_SECONDS:-300}" -O "$dst" "$url"
    return
  fi
  if command -v wget >/dev/null 2>&1; then
    wget -T "${VPSMONITOR_DOWNLOAD_TIMEOUT_SECONDS:-300}" -O "$dst" "$url"
    return
  fi
  die "curl, uclient-fetch, or wget is required to download packages."
}

realm_binary() {
  candidate="$(command -v realm 2>/dev/null || true)"
  if [ -n "$candidate" ] && [ -x "$candidate" ]; then
    printf '%s\n' "$candidate"
    return 0
  fi
  for candidate in /usr/bin/realm /usr/local/bin/realm /opt/realm/realm; do
    if [ -x "$candidate" ]; then
      printf '%s\n' "$candidate"
      return 0
    fi
  done
  return 1
}

realm_target() {
  case "$1" in
    amd64) printf '%s\n' x86_64-unknown-linux-musl ;;
    arm64) printf '%s\n' aarch64-unknown-linux-musl ;;
    arm) printf '%s\n' armv7-unknown-linux-musleabihf ;;
    *) return 1 ;;
  esac
}

install_realm_binary() {
  arch="$1"
  realm_version="${VPSMONITOR_REALM_VERSION:-v2.9.4}"
  target="$(realm_target "$arch")" || return 1
  package_name="realm-${target}.tar.gz"
  base_url="${VPSMONITOR_REALM_DOWNLOAD_BASE_URL:-https://github.com/zhboner/realm/releases/download/${realm_version}}"
  package_url="${VPSMONITOR_REALM_PACKAGE_URL:-${base_url%/}/${package_name}}"
  install_path="${VPSMONITOR_REALM_BINARY_PATH:-/usr/bin/realm}"
  tmp_dir="${tmp_dir:-$(make_temp_dir)}"
  work_dir="$tmp_dir/realm-install"
  package_path="$tmp_dir/$package_name"
  rm -rf "$work_dir"
  mkdir -p "$work_dir" "$(dirname "$install_path")"
  info "Downloading Realm $realm_version for $target:"
  info "  $package_url"
  download_file "$package_url" "$package_path" || return 1
  tar -xzf "$package_path" -C "$work_dir" || return 1
  [ -f "$work_dir/realm" ] || return 1
  cp "$work_dir/realm" "$install_path.new" || return 1
  chmod 755 "$install_path.new"
  mv -f "$install_path.new" "$install_path"
  info "Realm installed: $install_path"
  "$install_path" -v 2>/dev/null || true
}

install_realm_if_enabled() {
  arch="$1"
  is_truthy "${VPSMONITOR_REALM_AUTO_INSTALL:-false}" || return 0
  existing="$(realm_binary 2>/dev/null || true)"
  if [ -n "$existing" ] && ! is_truthy "${VPSMONITOR_REALM_FORCE_INSTALL:-false}"; then
    info "Keeping existing Realm binary: $existing"
    return 0
  fi
  if ! install_realm_binary "$arch"; then
    if is_truthy "${VPSMONITOR_REALM_REQUIRED:-false}"; then
      die "Realm installation failed. Set VPSMONITOR_REALM_DOWNLOAD_BASE_URL to a reachable mirror and retry."
    fi
    warn "Realm installation failed; bridge-client installation will continue."
  fi
}

install_haproxy_if_enabled() {
  is_truthy "${VPSMONITOR_HAPROXY_AUTO_INSTALL:-false}" || return 0
  if command -v haproxy >/dev/null 2>&1; then
    info "Keeping existing HAProxy binary: $(command -v haproxy)"
    return 0
  fi
  info "Installing HAProxy with opkg..."
  if opkg update && opkg install haproxy && command -v haproxy >/dev/null 2>&1; then
    info "HAProxy installed: $(command -v haproxy)"
    return 0
  fi
  if is_truthy "${VPSMONITOR_HAPROXY_REQUIRED:-false}"; then
    die "HAProxy installation failed."
  fi
  warn "HAProxy installation failed; bridge-client installation will continue."
}

make_temp_dir() {
  base="${VPSMONITOR_TMP_DIR:-/tmp}"
  mkdir -p "$base"
  mktemp -d "$base/vpsmonitor.XXXXXX" 2>/dev/null || die "Cannot create a temporary directory under $base."
}

random_token() {
  length="${1:-32}"
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex "$length" | cut -c "1-$length"
  else
    dd if=/dev/urandom bs="$length" count=1 2>/dev/null | od -An -tx1 | tr -d ' \n' | cut -c "1-$length"
  fi
}

sanitize_agent_id() {
  value="$(lower "$1" | sed 's/[^a-z0-9._-]/-/g; s/-\{2,\}/-/g; s/^-//; s/-$//')"
  [ -n "$value" ] || value="bridge-client"
  printf '%s' "$value" | cut -c 1-80 | sed 's/-$//'
}

default_client_agent_id() {
  host="$(hostname 2>/dev/null || echo openwrt)"
  printf '%s-%s\n' "$(sanitize_agent_id "$host")" "$(random_token 8)"
}

json_escape() {
  printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g; s/\r//g; s/\t/\\t/g'
}

json_bool() {
  if is_truthy "$1"; then echo true; else echo false; fi
}

read_json_string_field() {
  file="$1"
  key="$2"
  [ -f "$file" ] || return 0
  sed -n 's/^[[:space:]]*"'"$key"'"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$file" | head -n 1
}

prompt_default() {
  label="$1"
  default="$2"
  if assume_yes; then
    printf '%s' "$default"
    return
  fi
  printf '%s [%s]: ' "$label" "$default" >&2
  IFS= read -r value || value=""
  printf '%s' "${value:-$default}"
}

prompt_required() {
  label="$1"
  default="${2:-}"
  if assume_yes; then
    [ -n "$default" ] || die "$label is required for non-interactive installation."
    printf '%s' "$default"
    return
  fi
  while :; do
    printf '%s: ' "$label" >&2
    IFS= read -r value || value=""
    if [ -n "$value" ]; then
      printf '%s' "$value"
      return
    fi
    warn "This value is required."
  done
}

absolute_path() {
  value="$1"
  [ -n "$value" ] || die "Path cannot be empty."
  case "$value" in
    /*) printf '%s\n' "${value%/}" ;;
    *) printf '%s/%s\n' "$(pwd -P)" "${value%/}" ;;
  esac
}

package_urls() {
  arch="$1"
  package_name="${package_prefix}-client-linux-${arch}.tar.gz"
  if [ -n "${VPSMONITOR_CLIENT_PACKAGE_URL:-}" ]; then
    printf '%s\n' "$VPSMONITOR_CLIENT_PACKAGE_URL"
  elif [ -n "${VPSMONITOR_PACKAGE_URL:-}" ]; then
    printf '%s\n' "$VPSMONITOR_PACKAGE_URL"
  elif [ -n "${VPSMONITOR_BASE_URL:-}" ]; then
    printf '%s/%s\n' "${VPSMONITOR_BASE_URL%/}" "$package_name"
  elif [ "$version" = "latest" ]; then
    printf '%s\n' \
      "https://github.com/${repo}/releases/latest/download/${package_name}" \
      "https://cdn.jsdelivr.net/gh/${repo}@main/dist/${package_name}" \
      "https://raw.githubusercontent.com/${repo}/main/dist/${package_name}"
  else
    printf '%s\n' \
      "https://github.com/${repo}/releases/download/${version}/${package_name}" \
      "https://cdn.jsdelivr.net/gh/${repo}@${version}/dist/${package_name}" \
      "https://raw.githubusercontent.com/${repo}/${version}/dist/${package_name}"
  fi
}

fetch_bundle() {
  arch="$1"
  tmp_dir="${tmp_dir:-$(make_temp_dir)}"
  package_path="$tmp_dir/client.tar.gz"
  urls_path="$tmp_dir/package-urls"
  package_urls "$arch" >"$urls_path"
  while IFS= read -r url; do
    [ -n "$url" ] || continue
    info "Downloading bridge-client package: $url" >&2
    rm -f "$package_path"
    if download_file "$url" "$package_path"; then
      printf '%s\n' "$url" >"$tmp_dir/downloaded-url"
      break
    fi
    warn "Download source failed, trying the next source."
  done <"$urls_path"
  [ -f "$tmp_dir/downloaded-url" ] || die "Download failed. Set VPSMONITOR_CLIENT_PACKAGE_URL or VPSMONITOR_BASE_URL and retry."
  tar -xzf "$package_path" -C "$tmp_dir"
  binary="$(find "$tmp_dir" -type f -name bridge-client | head -n 1)"
  [ -n "$binary" ] || die "bridge-client binary was not found in package."
  bundle_dir="$(dirname "$binary")"
}

write_client_config() {
  config_path="$1"
  server_url="$2"
  agent_id="$3"
  registration_token="$4"
  skip_tls_verify="$5"
  poll_interval="$6"
  request_timeout="$7"
  mkdir -p "$(dirname "$config_path")"
  umask 077
  {
    printf '{\n'
    printf '  "agent_id": "%s",\n' "$(json_escape "$agent_id")"
    printf '  "registration_token": "%s",\n' "$(json_escape "$registration_token")"
    printf '  "agent_token": "",\n'
    printf '  "server_url": "%s",\n' "$(json_escape "$server_url")"
    printf '  "server_skip_tls_verify": %s,\n' "$(json_bool "$skip_tls_verify")"
    printf '  "poll_interval": "%s",\n' "$(json_escape "$poll_interval")"
    printf '  "request_timeout_seconds": %s\n' "$request_timeout"
    printf '}\n'
  } >"$config_path"
  chmod 600 "$config_path"
}

install_procd_service() {
  service_name="$1"
  install_dir="$2"
  config_path="$3"
  service_file="/etc/init.d/$service_name"
  cat >"$service_file" <<EOF
#!/bin/sh /etc/rc.common

START=95
STOP=10
USE_PROCD=1

start_service() {
  procd_open_instance
  procd_set_param command "$install_dir/bridge-client" -config "$config_path"
  procd_set_param respawn 3600 5 5
  procd_set_param stdout 1
  procd_set_param stderr 1
  procd_set_param file "$config_path"
  procd_set_param env PATH=/usr/sbin:/usr/bin:/sbin:/bin
  procd_close_instance
}

service_triggers() {
  procd_add_reload_trigger network
}
EOF
  chmod 755 "$service_file"
  "$service_file" enable
  "$service_file" restart || "$service_file" start
  sleep 1
  "$service_file" running || die "failed to start $service_name under procd. Run: logread -e $service_name"
}

install_client() {
  arch="$1"
  install_dir="$(absolute_path "${VPSMONITOR_CLIENT_DIR:-$prefix/client}")"
  config_path="$install_dir/config/client.json"
  server_url="${VPSMONITOR_SERVER_URL:-http://SERVER_IP:8090}"
  registration_token="${VPSMONITOR_REGISTRATION_TOKEN:-}"
  skip_tls_verify="${VPSMONITOR_SERVER_SKIP_TLS_VERIFY:-false}"
  poll_interval="${VPSMONITOR_POLL_INTERVAL:-30s}"
  request_timeout="${VPSMONITOR_REQUEST_TIMEOUT_SECONDS:-15}"
  agent_id="${VPSMONITOR_AGENT_ID:-}"
  existing_agent_id="$(read_json_string_field "$config_path" agent_id)"
  [ -n "$agent_id" ] || agent_id="$existing_agent_id"
  [ -n "$agent_id" ] || agent_id="$(default_client_agent_id)"
  agent_id="$(sanitize_agent_id "$agent_id")"

  if [ -f "$config_path" ] && ! is_truthy "${VPSMONITOR_FORCE_CONFIG:-}"; then
    info "Keeping existing client config: $config_path"
  else
    server_url="$(prompt_default "Server URL" "$server_url")"
    registration_token="$(prompt_required "Client registration token" "$registration_token")"
    agent_id="$(sanitize_agent_id "$(prompt_default "Client ID" "$agent_id")")"
    skip_tls_verify="$(prompt_default "Skip server TLS verification? true/false" "$skip_tls_verify")"
    poll_interval="$(prompt_default "Poll interval" "$poll_interval")"
    request_timeout="$(prompt_default "Request timeout seconds" "$request_timeout")"
    write_client_config "$config_path" "$server_url" "$agent_id" "$registration_token" "$skip_tls_verify" "$poll_interval" "$request_timeout"
  fi

  fetch_bundle "$arch"
  source_dir="$bundle_dir"
  mkdir -p "$install_dir"
  cp "$source_dir/bridge-client" "$install_dir/.bridge-client.new"
  chmod 755 "$install_dir/.bridge-client.new"
  mv -f "$install_dir/.bridge-client.new" "$install_dir/bridge-client"
  [ ! -f "$source_dir/README.md" ] || cp "$source_dir/README.md" "$install_dir/README.md"

  install_realm_if_enabled "$arch"
  install_haproxy_if_enabled

  service_name="${VPSMONITOR_CLIENT_SERVICE:-vpsmonitor-client}"
  install_procd_service "$service_name" "$install_dir" "$config_path"

  info "bridge-client installed."
  info "  Platform: OpenWrt/iStoreOS linux/$arch"
  info "  Service:  $service_name (procd)"
  info "  Config:   $config_path"
  info "  Client ID: $(read_json_string_field "$config_path" agent_id)"
  info "  Status:   /etc/init.d/$service_name status"
  info "  Logs:     logread -f -e $service_name"
}

usage() {
  cat <<EOF
Usage:
  sh install-openwrt.sh client

Supported architectures: x86_64, arm64/aarch64, armv7
Service manager: OpenWrt procd

Realm / HAProxy overrides:
  VPSMONITOR_REALM_AUTO_INSTALL=false
  VPSMONITOR_HAPROXY_AUTO_INSTALL=false
  VPSMONITOR_REALM_VERSION=v2.9.4
  VPSMONITOR_REALM_DOWNLOAD_BASE_URL=https://example.com/realm/v2.9.4
  VPSMONITOR_REALM_PACKAGE_URL=https://example.com/realm-x86_64-unknown-linux-musl.tar.gz
EOF
}

main() {
  action="${1:-client}"
  case "$action" in
    -h|--help) usage; exit 0 ;;
    client) ;;
    *) die "OpenWrt/iStoreOS supports the client target only." ;;
  esac
  require_openwrt
  arch="$(detect_arch)"
  info "VPSMonitor OpenWrt/iStoreOS installer"
  info "  Arch: linux/$arch"
  info "  Repo: $repo"
  info "  Version: $version"
  install_client "$arch"
}

main "$@"
