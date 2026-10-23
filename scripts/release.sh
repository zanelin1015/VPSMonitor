#!/usr/bin/env bash
# Build, verify and publish a unified Server/Client release. Compatible with Bash 3.2.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
START_DIR="$PWD"
VERSION=""
TITLE=""
NOTES_SOURCE=""
DRY_RUN=0
ASSUME_YES=0
DRAFT_ONLY=0
WORK_DIR=""
STEP="准备"
TARGETS=(linux/amd64 linux/arm64 linux/arm windows/amd64 windows/arm64)

usage() {
  cat <<'HELP'
用法：./scripts/release.sh [选项]

默认交互输入版本号和多行发布说明，测试、打包后确认发布到 origin 对应的 GitHub 仓库。
功能代码须先提交；脚本只自动提交版本文件和内嵌前端资源。

  --version X.Y.Z       指定版本号（也接受 vX.Y.Z）
  --notes-file PATH     从 Markdown 文件读取发布说明
  --title TEXT          Release 标题，默认 vX.Y.Z
  --draft               上传为草稿，暂不公开发布
  --yes                 跳过发布确认，必须同时指定版本号和说明文件
  --dry-run             只预览计划；不改文件、不联网、不构建、不提交、不发布
  -h, --help            显示帮助

示例：
  ./scripts/release.sh
  ./scripts/release.sh --version 0.3.55 --notes-file /tmp/release-notes.md
  ./scripts/release.sh --version 0.3.55 --notes-file /tmp/release-notes.md --yes
  ./scripts/release.sh --dry-run
HELP
}

die() { printf '错误：%s\n' "$*" >&2; exit 1; }
step() { STEP="$1"; printf '\n==> %s\n' "$STEP"; }
need_value() { [[ "$#" -ge 2 && -n "$2" && "$2" != --* ]] || die "$1 需要参数"; }
while [[ "$#" -gt 0 ]]; do
  case "$1" in
    --version) need_value "$@"; VERSION="${2#v}"; shift 2 ;;
    --notes-file) need_value "$@"; NOTES_SOURCE="$2"; shift 2 ;;
    --title) need_value "$@"; TITLE="$2"; shift 2 ;;
    --dry-run) DRY_RUN=1; shift ;;
    --yes) ASSUME_YES=1; shift ;;
    --draft) DRAFT_ONLY=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "未知选项：$1（使用 --help 查看帮助）" ;;
  esac
done

cleanup() {
  local status=$?
  [[ -z "$WORK_DIR" ]] || rm -rf "$WORK_DIR"
  if [[ "$status" -ne 0 ]]; then
    printf '\n发布已停止，阶段：%s。不会继续执行后续步骤。\n' "$STEP" >&2
    printf '已生成的 dist 文件、本地提交和标签会保留；请先检查失败原因再重试。\n' >&2
    printf '若 GitHub 上已创建草稿，请在草稿中检查资产后继续发布；脚本不会覆盖已有标签或 Release。\n' >&2
  fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

valid_version() {
  local value="$1" part
  [[ "$value" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || return 1
  local parts
  IFS=. read -r -a parts <<< "$value"
  for part in "${parts[@]}"; do [[ "${#part}" -le 9 ]] || return 1; done
}
version_greater() {
  local left right index
  IFS=. read -r -a left <<< "$1"
  IFS=. read -r -a right <<< "$2"
  for index in 0 1 2; do
    if (( left[index] > right[index] )); then return 0; fi
    if (( left[index] < right[index] )); then return 1; fi
  done
  return 1
}
source_is_clean() {
  # Older revisions track dist archives despite .gitignore. Packaging rewrites
  # these outputs; they are release assets rather than source modifications.
  git diff --quiet -- . ':(exclude)dist/**' && git diff --cached --quiet || return 1
  # These are local screenshot/analysis files, never release source inputs.
  [[ -z "$(git ls-files --others --exclude-standard -- . ':(exclude).DS_Store' ':(exclude)**/.DS_Store' ':(exclude)artifacts/**')" ]]
}

cd "$ROOT_DIR"
[[ "$(git rev-parse --show-toplevel)" == "$ROOT_DIR" ]] || die "脚本必须位于项目 Git 仓库的 scripts 目录"
if [[ "$ASSUME_YES" -eq 1 && ( -z "$VERSION" || -z "$NOTES_SOURCE" ) ]]; then
  die "--yes 必须同时提供 --version 和 --notes-file"
fi
current_version="0.0.0"
for file in VERSION VERSION.server VERSION.client; do
  [[ -f "$file" ]] || die "缺少 $file"
  value="$(tr -d '[:space:]' < "$file")"
  valid_version "$value" || die "$file 中的版本无效：$value"
  if version_greater "$value" "$current_version"; then current_version="$value"; fi
done
suggested="${current_version%.*}.$((${current_version##*.} + 1))"
if [[ -z "$VERSION" ]]; then
  if [[ "$DRY_RUN" -eq 0 ]]; then
    [[ -t 0 ]] || die "非交互运行请提供 --version 和 --notes-file"
    read -r -p "新版本号 [$suggested]：" VERSION
  fi
  VERSION="${VERSION:-$suggested}"
  VERSION="${VERSION#v}"
fi
valid_version "$VERSION" || die "版本号必须是 X.Y.Z，不能有前导零"
version_greater "$VERSION" "$current_version" || die "新版本必须高于当前 Server/Client 版本 $current_version"
TAG="v$VERSION"
TITLE="${TITLE:-$TAG}"
NOTES_FILE="$ROOT_DIR/dist/release-$TAG.md"
if [[ -n "$NOTES_SOURCE" ]]; then
  [[ "$NOTES_SOURCE" == /* ]] || NOTES_SOURCE="$START_DIR/$NOTES_SOURCE"
  [[ -f "$NOTES_SOURCE" && -r "$NOTES_SOURCE" ]] || die "无法读取说明文件：$NOTES_SOURCE"
  notes="$(cat "$NOTES_SOURCE")"
  [[ -n "${notes//[[:space:]]/}" ]] || die "发布说明不能为空"
fi

if [[ "$DRY_RUN" -eq 1 ]]; then
  printf '预览：%s → %s\n标题：%s\n仓库：%s\n' "$current_version" "$TAG" "$TITLE" "$(git remote get-url origin)"
  if ! source_is_clean; then printf '正式执行前需要先提交功能代码（当前工作区有未提交文件）。\n'; fi
  printf '%s\n' '步骤：检查远端 → 输入发布说明 → 前端回归与构建 → 提交版本及前端资源 → Go 测试及 10 个安装包 → 校验版本与 SHA-256 → 确认 → 推送分支/标签 → 上传并核对草稿资产'
  if [[ "$DRAFT_ONLY" -eq 0 ]]; then printf '最后公开发布并标记为 Latest。\n'; else printf '保留草稿，不公开发布。\n'; fi
  [[ -z "$NOTES_SOURCE" ]] || printf '\n发布说明：\n%s\n' "$notes"
  exit 0
fi

step "环境与远端检查"
[[ "$ASSUME_YES" -eq 1 || -t 0 ]] || die "非交互发布请同时提供 --version、--notes-file 和 --yes"
for command in git go node npm gh tar zip unzip cmp shasum; do
  command -v "$command" >/dev/null 2>&1 || die "缺少命令：$command"
done
source_is_clean || die "工作区有未提交的功能代码；请先用 git status 检查并提交需要发布的文件，然后重新运行"
branch="$(git symbolic-ref --quiet --short HEAD)" || die "不能从 detached HEAD 发布"
gh auth status >/dev/null
repo_info="$(gh repo view "$(git remote get-url origin)" --json nameWithOwner,defaultBranchRef --jq '[.nameWithOwner, .defaultBranchRef.name] | @tsv')"
IFS=$'\t' read -r REPO DEFAULT_BRANCH <<< "$repo_info"
[[ -n "$REPO" && -n "$DEFAULT_BRANCH" ]] || die "无法识别 GitHub 仓库或默认分支"
[[ "$branch" == "$DEFAULT_BRANCH" ]] || die "请切换到默认分支 $DEFAULT_BRANCH 后发布（当前为 ${branch}）"
git fetch origin "refs/heads/$branch:refs/remotes/origin/$branch" --tags
git merge-base --is-ancestor "refs/remotes/origin/$branch" HEAD || die "本地分支落后或已分叉，请先整合 origin/$branch"
if git show-ref --verify --quiet "refs/tags/$TAG"; then die "标签 $TAG 已存在，不会覆盖"; fi
existing_releases="$(gh release list --repo "$REPO" --limit 100 --json tagName --jq '.[].tagName')"
while IFS= read -r existing; do
  [[ "$existing" != "$TAG" ]] || die "Release $TAG 已存在，不会覆盖"
done <<< "$existing_releases"

step "填写发布说明"
mkdir -p "$ROOT_DIR/dist"
if [[ -n "$NOTES_SOURCE" ]]; then
  if [[ "$NOTES_SOURCE" != "$NOTES_FILE" ]]; then cp "$NOTES_SOURCE" "$NOTES_FILE"; fi
else
  [[ -t 0 ]] || die "非交互运行请使用 --notes-file"
  printf '输入 Markdown 发布说明，可输入多行；单独输入一个 . 后回车结束：\n'
  : > "$NOTES_FILE"
  while IFS= read -r line; do
    [[ "$line" != '.' ]] || break
    printf '%s\n' "$line" >> "$NOTES_FILE"
  done
fi
notes="$(cat "$NOTES_FILE")"
[[ -n "${notes//[[:space:]]/}" ]] || die "发布说明不能为空"

step "前端测试与构建"
(cd web && npm ci)
test_scripts="$(node -e 'for (const key of Object.keys(require("./web/package.json").scripts || {})) if (key.startsWith("test:")) console.log(key)')"
while IFS= read -r test_script; do
  [[ -z "$test_script" ]] || (cd web && npm run "$test_script")
done <<< "$test_scripts"
for file in VERSION VERSION.server VERSION.client; do printf '%s\n' "$VERSION" > "$file"; done
(cd web && npm run build)

step "提交版本与前端资源"
git add -- VERSION VERSION.server VERSION.client webui/dist
git diff --cached --check -- . ':(exclude)webui/dist/**'
git commit -m "Release $TAG"
source_is_clean || die "构建过程中出现额外源码变化，请检查后再发布"
RELEASE_COMMIT="$(git rev-parse HEAD)"

step "Go 测试及多平台打包"
# Versions and the embedded UI were committed before packaging so every binary
# carries the exact release commit. Override component-specific environment too.
VPSMONITOR_BUILD_VERSION="$VERSION" VPSMONITOR_SERVER_BUILD_VERSION="$VERSION" \
  VPSMONITOR_CLIENT_BUILD_VERSION="$VERSION" VPSMONITOR_SKIP_WEB_BUILD=1 \
  PACKAGE_PREFIX=VPSMonitor bash scripts/build.sh "${TARGETS[@]}"
source_is_clean || die "打包改变了已提交源码，请检查；本次不会推送或发布"

step "安装包版本与校验文件验证"
WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/vpsmonitor-release.XXXXXX")"
ASSETS=()
for target in "${TARGETS[@]}"; do
  os="${target%/*}"; arch="${target#*/}"
  for role in server client; do
    package="VPSMonitor-$role-$os-$arch"
    ext=tar.gz; binary="bridge-$role"
    if [[ "$os" == windows ]]; then ext=zip; binary="$binary.exe"; fi
    archive="$ROOT_DIR/dist/$package.$ext"
    [[ -s "$archive" && ! -L "$archive" ]] || die "缺少安装包或安装包为空：$archive"
    if [[ "$os" == windows ]]; then
      packaged_version="$(unzip -p "$archive" "$package/VERSION")"
      unzip -p "$archive" "$package/$binary" > "$WORK_DIR/binary"
    else
      packaged_version="$(tar -xOzf "$archive" "$package/VERSION")"
      tar -xOzf "$archive" "$package/$binary" > "$WORK_DIR/binary"
    fi
    [[ "$packaged_version" == "$VERSION" ]] || die "安装包版本不一致：$archive"
    metadata="$(go version -m "$WORK_DIR/binary")"$'\n'
    # -trimpath may omit ldflags from Go build info. Use Go's VCS stamp
    # for the full commit identity and the archive VERSION for the version.
    [[ "$metadata" == *$'\tvcs.revision='"$RELEASE_COMMIT"$'\n'* && "$metadata" == *$'\tGOOS='"$os"$'\n'* && "$metadata" == *$'\tGOARCH='"$arch"$'\n'* && "$metadata" == *$'\tpath\tbridge-core/cmd/bridge-'"$role"$'\n'* ]] || die "二进制提交、平台或组件不一致：$archive"
    ASSETS+=("$archive")
  done
done
(
  cd "$ROOT_DIR/dist"
  for asset in "${ASSETS[@]}"; do shasum -a 256 "${asset##*/}"; done > checksums.txt
  shasum -a 256 --check checksums.txt
)
ASSETS+=("$ROOT_DIR/dist/checksums.txt")
printf '\n仓库：%s\n版本：%s\n提交：%s\n标题：%s\n资产：10 个安装包 + checksums.txt\n发布说明：\n%s\n\n' "$REPO" "$TAG" "$RELEASE_COMMIT" "$TITLE" "$notes"
if [[ "$ASSUME_YES" -eq 0 ]]; then
  [[ -t 0 ]] || die "非交互发布请使用 --yes"
  read -r -p '以上内容已准备好，确认推送并创建 GitHub Release？[y/N] ' answer
  case "$answer" in y|Y|yes|YES) ;; *) printf '已取消发布；本地提交及安装包保留。\n'; exit 0 ;; esac
fi

step "推送分支与版本标签"
[[ "$(git rev-parse HEAD)" == "$RELEASE_COMMIT" ]] && source_is_clean || die "准备期间源码或提交已变化，请检查后再发布"
(cd "$ROOT_DIR/dist" && shasum -a 256 --check checksums.txt)
git tag -a "$TAG" -m "Release $TAG" "$RELEASE_COMMIT"
git push --atomic origin "$RELEASE_COMMIT:refs/heads/$branch" "refs/tags/$TAG"

step "上传并核对 GitHub 草稿"
gh release create "$TAG" "${ASSETS[@]}" --repo "$REPO" --verify-tag --draft --title "$TITLE" --notes-file "$NOTES_FILE"
remote_names="$(gh release view "$TAG" --repo "$REPO" --json assets --jq '.assets[].name')"
remote_count=0
while IFS= read -r name; do [[ -z "$name" ]] || remote_count=$((remote_count + 1)); done <<< "$remote_names"
[[ "$remote_count" -eq "${#ASSETS[@]}" ]] || die "远端资产数量不符，Release 已保留为草稿"
for asset in "${ASSETS[@]}"; do
  found=0
  while IFS= read -r name; do if [[ "$name" == "${asset##*/}" ]]; then found=1; break; fi; done <<< "$remote_names"
  [[ "$found" -eq 1 ]] || die "远端缺少资产：${asset##*/}，Release 已保留为草稿"
done
gh release download "$TAG" --repo "$REPO" --pattern checksums.txt --dir "$WORK_DIR"
cmp "$ROOT_DIR/dist/checksums.txt" "$WORK_DIR/checksums.txt" || die "远端校验文件不一致，Release 已保留为草稿"
if [[ "$DRAFT_ONLY" -eq 0 ]]; then
  step "公开发布"
  gh release edit "$TAG" --repo "$REPO" --draft=false --latest
fi
gh release view "$TAG" --repo "$REPO" --json url,isDraft --jq '"Release: " + .url + " (draft=" + (.isDraft|tostring) + ")"'
printf '完成。安装包、checksums.txt 和发布说明保存在 %s/dist/\n' "$ROOT_DIR"
