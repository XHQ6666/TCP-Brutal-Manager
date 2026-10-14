#!/usr/bin/env bash
set -euo pipefail

project_root=$(cd "$(dirname "$0")/.." && pwd)
cd "$project_root"
out=${1:-"$project_root/dist"}
if [ "$#" -gt 0 ]; then shift; fi
version=${VERSION:-1.0.1}
source_commit=${BUILD_COMMIT:-${GITHUB_SHA:-}}
if [ -z "$source_commit" ]; then
  source_commit=$(git rev-parse HEAD 2>/dev/null || true)
  if [ -n "$source_commit" ] && [ -n "$(git status --porcelain)" ]; then
    echo 'Refusing to label uncommitted source with a Git commit. Commit the source or supply BUILD_COMMIT explicitly.' >&2
    exit 1
  fi
fi
if [[ ! "$source_commit" =~ ^[0-9a-fA-F]{40}$ ]]; then
  echo 'A 40-character source commit is required (Git checkout, GITHUB_SHA or BUILD_COMMIT).' >&2
  exit 1
fi
build_time=${BUILD_TIME:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}
if [[ ! "$version" =~ ^[A-Za-z0-9][A-Za-z0-9.+_-]*$ ]] || [[ ! "$build_time" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$ ]]; then
  echo 'Invalid VERSION or UTC BUILD_TIME.' >&2
  exit 1
fi
if [ "$#" -eq 0 ]; then set -- amd64 arm64 386 armv5 armv6 armv7; fi
archive_version=$(printf '%s' "$version" | tr '[:upper:]' '[:lower:]')
flags="-X main.version=$version -X main.commit=$source_commit -X main.buildTime=$build_time"
mkdir -p "$out"
archives=()
for target in "$@"; do
  goarm=''
  case "$target" in
    amd64|arm64|386) goarch=$target ;;
    armv5|armv6|armv7) goarch=arm; goarm=${target#armv} ;;
    *) echo "Unsupported Linux target: $target" >&2; exit 1 ;;
  esac
  name="tcp-brutal-manager-$archive_version-linux-$target"
  package_dir="$out/$name"
  mkdir -p "$package_dir"
  CGO_ENABLED=0 GOOS=linux GOARCH="$goarch" GOARM="$goarm" go build -trimpath -ldflags "$flags" -o "$package_dir/tcp-brutal-manager" ./cmd/tcp-brutal-manager
  cp README.md "$package_dir/README.md"
  cp examples/config.json "$package_dir/config.example.json"
  cp systemd/tcp-brutal-manager.service "$package_dir/tcp-brutal-manager.service"
  cp RELEASE_NOTES.md "$package_dir/RELEASE_NOTES.md"
  mkdir -p "$package_dir/docs"
  cp docs/CONFIGURATION.md docs/CLI.md docs/SOP.md "$package_dir/docs/"
  COPYFILE_DISABLE=1 tar --no-xattrs -czf "$out/$name.tar.gz" -C "$out" "$name"
  archives+=("$name.tar.gz")
  printf 'Built %s commit=%s built=%s\n' "$target" "$source_commit" "$build_time"
done
if command -v sha256sum >/dev/null 2>&1; then
  (cd "$out" && sha256sum "${archives[@]}" > SHA256SUMS)
else
  (cd "$out" && shasum -a 256 "${archives[@]}" > SHA256SUMS)
fi
