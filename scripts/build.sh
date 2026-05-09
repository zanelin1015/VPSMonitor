#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_DIR="${ROOT_DIR}/dist"
CACHE_DIR="${ROOT_DIR}/.cache/go-build"

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

  local ext=""
  if [[ "$goos" == "windows" ]]; then
    ext=".exe"
  fi

  local package_name="${app_name}-${goos}-${goarch}"
  local output_dir="${DIST_DIR}/${package_name}"
  local config_src="${ROOT_DIR}/config/${app_name#bridge-}.example.json"
  local config_dst="${output_dir}/config/${app_name#bridge-}.json"
  local binary_path="${output_dir}/${app_name}${ext}"

  rm -rf "$output_dir"
  mkdir -p "${output_dir}/config"

  GOOS="$goos" GOARCH="$goarch" go build -trimpath -ldflags="-s -w" -o "$binary_path" "$entrypoint"

  cp "$config_src" "$config_dst"
  cp "${ROOT_DIR}/README.md" "${output_dir}/README.md"

  if [[ "$goos" == "windows" ]]; then
    cp "${ROOT_DIR}/scripts/templates/run-${app_name}.bat" "${output_dir}/run.bat"
    (
      cd "$DIST_DIR"
      zip -qr "${package_name}.zip" "$package_name"
    )
  else
    cp "${ROOT_DIR}/scripts/templates/run-${app_name}.sh" "${output_dir}/run.sh"
    if [[ "$app_name" == "bridge-server" ]]; then
      cp "${ROOT_DIR}/scripts/templates/install-bridge-server.sh" "${output_dir}/install.sh"
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

echo "build artifacts written to $DIST_DIR"
