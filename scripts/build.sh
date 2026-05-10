#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_DIR="${ROOT_DIR}/dist"
CACHE_DIR="${ROOT_DIR}/.cache/go-build"
VERSION_FILE="${ROOT_DIR}/VERSION"
SERVER_VERSION_FILE="${ROOT_DIR}/VERSION.server"
CLIENT_VERSION_FILE="${ROOT_DIR}/VERSION.client"
VERSION_PKG="bridge-core/internal/version"


semver_patch_bump() {
  local version="$1"
  if [[ ! "$version" =~ ^([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
    echo "invalid semantic version: $version" >&2
    return 1
  fi
  printf "%s.%s.%s" "${BASH_REMATCH[1]}" "${BASH_REMATCH[2]}" "$((BASH_REMATCH[3] + 1))"
}

resolve_component_version() {
  local role="$1"
  local version_file="$2"
  local env_name="$3"
  local requested="${VPSMONITOR_BUILD_VERSION:-}"
  local role_requested="${!env_name:-}"

  if [[ -n "$role_requested" ]]; then
    requested="$role_requested"
  fi

  if [[ -n "$requested" ]]; then
    if [[ ! "$requested" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
      echo "$env_name must use MAJOR.MINOR.PATCH, got: $requested" >&2
      return 1
    fi
    printf "%s\n" "$requested" >"$version_file"
    echo "$requested"
    return 0
  fi

  if [[ ! -f "$version_file" ]]; then
    if [[ -f "$VERSION_FILE" ]]; then
      tr -d '[:space:]' <"$VERSION_FILE" >"$version_file"
    else
      echo "0.1.0" >"$version_file"
    fi
  fi

  local current
  current="$(tr -d '[:space:]' <"$version_file")"
  local next
  next="$(semver_patch_bump "$current")"
  printf "%s\n" "$next" >"$version_file"
  echo "$next"
}

SERVER_BUILD_VERSION="$(resolve_component_version server "$SERVER_VERSION_FILE" VPSMONITOR_SERVER_BUILD_VERSION)"
CLIENT_BUILD_VERSION="$(resolve_component_version client "$CLIENT_VERSION_FILE" VPSMONITOR_CLIENT_BUILD_VERSION)"
printf "%s\n" "$SERVER_BUILD_VERSION" >"$VERSION_FILE"
BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
GIT_COMMIT="$(git -C "$ROOT_DIR" rev-parse --short HEAD 2>/dev/null || true)"

export GOCACHE="${GOCACHE:-$CACHE_DIR}"
export CGO_ENABLED=0

if [[ "$#" -gt 0 ]]; then
  TARGETS=("$@")
else
  TARGETS=("linux/amd64" "windows/amd64")
fi

mkdir -p "$DIST_DIR" "$GOCACHE"

if ! command -v npm >/dev/null 2>&1; then
  echo "npm is required to build the embedded web console" >&2
  exit 1
fi

package_component() {
  local app_name="$1"
  local entrypoint="$2"
  local goos="$3"
  local goarch="$4"
  local package_role="${app_name#bridge-}"
  local package_prefix="${PACKAGE_PREFIX:-VPSMonitor}"
  local build_version="$SERVER_BUILD_VERSION"
  local version_src="$SERVER_VERSION_FILE"
  if [[ "$package_role" == "client" ]]; then
    build_version="$CLIENT_BUILD_VERSION"
    version_src="$CLIENT_VERSION_FILE"
  fi
  local go_ldflags="-s -w -X ${VERSION_PKG}.Version=${build_version} -X ${VERSION_PKG}.BuildTime=${BUILD_TIME} -X ${VERSION_PKG}.GitCommit=${GIT_COMMIT}"

  local ext=""
  if [[ "$goos" == "windows" ]]; then
    ext=".exe"
  fi

  local package_name="${package_prefix}-${package_role}-${goos}-${goarch}"
  local output_dir="${DIST_DIR}/${package_name}"
  local config_src="${ROOT_DIR}/config/${package_role}.example.json"
  local config_dst="${output_dir}/config/${package_role}.json"
  local binary_path="${output_dir}/${app_name}${ext}"

  rm -rf "$output_dir"
  rm -f "${DIST_DIR}/${package_name}.tar.gz" "${DIST_DIR}/${package_name}.zip"
  rm -rf "${DIST_DIR}/${app_name}-${goos}-${goarch}"
  rm -f "${DIST_DIR}/${app_name}-${goos}-${goarch}.tar.gz" "${DIST_DIR}/${app_name}-${goos}-${goarch}.zip"
  mkdir -p "${output_dir}/config"

  GOOS="$goos" GOARCH="$goarch" go build -trimpath -ldflags="$go_ldflags" -o "$binary_path" "$entrypoint"

  cp "$config_src" "$config_dst"
  cp "${ROOT_DIR}/README.md" "${output_dir}/README.md"
  cp "$version_src" "${output_dir}/VERSION"

  if [[ "$goos" == "windows" ]]; then
    cp "${ROOT_DIR}/scripts/templates/run-${app_name}.bat" "${output_dir}/run.bat"
    if [[ "$app_name" == "bridge-client" ]]; then
      cp "${ROOT_DIR}/install.ps1" "${output_dir}/install.ps1"
      cp "${ROOT_DIR}/install-client.cmd" "${output_dir}/install-client.cmd"
    fi
    (
      cd "$DIST_DIR"
      zip -qr "${package_name}.zip" "$package_name"
    )
  else
    cp "${ROOT_DIR}/scripts/templates/run-${app_name}.sh" "${output_dir}/run.sh"
    if [[ "$app_name" == "bridge-server" ]]; then
      cp "${ROOT_DIR}/install.sh" "${output_dir}/install.sh"
      chmod +x "${output_dir}/install.sh"
    fi
    chmod +x "${output_dir}/run.sh" "$binary_path"
    (
      cd "$DIST_DIR"
      tar -czf "${package_name}.tar.gz" "$package_name"
    )
  fi
}

cd "$ROOT_DIR"

if [[ ! -d "${ROOT_DIR}/web/node_modules" ]]; then
  (
    cd "${ROOT_DIR}/web"
    npm install
  )
fi

(
  cd "${ROOT_DIR}/web"
  npm run build
)

go test ./...

for target in "${TARGETS[@]}"; do
  GOOS="${target%/*}"
  GOARCH="${target#*/}"

  package_component "bridge-server" "./cmd/bridge-server" "$GOOS" "$GOARCH"
  package_component "bridge-client" "./cmd/bridge-client" "$GOOS" "$GOARCH"
done

echo "server version $SERVER_BUILD_VERSION"
echo "client version $CLIENT_BUILD_VERSION"
echo "build artifacts written to $DIST_DIR"
