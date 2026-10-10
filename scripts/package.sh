#!/bin/sh
set -eu

# Run from the repository root. Cross-build with GOOS and GOARCH.
release_version=${1:-dev}
output=${2:-dist}
case "$release_version" in
  ''|*[!A-Za-z0-9._-]*) echo 'Version must contain only letters, digits, dots, underscores or hyphens.' >&2; exit 2 ;;
esac
target_os=$(go env GOOS)
target_arch=$(go env GOARCH)
case "$target_os/$target_arch" in
  linux/amd64|linux/arm64|darwin/amd64|darwin/arm64|windows/amd64) ;;
  *) echo "Unsupported package target: $target_os/$target_arch" >&2; exit 2 ;;
esac
release_revision=$(git rev-parse HEAD)
release_date=$(git show -s --format=%cI HEAD)
case "$release_revision" in *[!a-f0-9]*) exit 2 ;; esac
case "$release_date" in *[!0-9T:+-]*) exit 2 ;; esac
source_state=clean
if [ -n "$(git status --porcelain --untracked-files=normal)" ]; then
  source_state=dirty
  release_revision="$release_revision-dirty"
fi
name="gitmoji_${release_version}_${target_os}_${target_arch}"
mkdir -p "$output"
output=$(cd "$output" && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT HUP INT TERM
stage="$work/$name"
mkdir -p "$stage/licenses"
binary=gitmoji
if [ "$target_os" = windows ]; then binary=gitmoji.exe; fi
CGO_ENABLED=0 go build -trimpath -buildvcs=false \
  -ldflags="-s -w -X main.version=$release_version -X main.revision=$release_revision -X main.buildDate=$release_date" \
  -o "$stage/$binary" ./cmd/gitmoji
cp LICENSE README.md "$stage/"
cp go.mod "$stage/"
cp internal/catalog/assets/LICENSE "$stage/licenses/gitmoji-catalog-LICENSE"
cp licenses/Go-LICENSE "$stage/licenses/Go-LICENSE"
# Include notices from modules actually linked for the requested target.
go list -deps -f '{{if and .Module (not .Module.Main)}}{{.Module.Path}}{{"\t"}}{{.Module.Version}}{{"\t"}}{{.Module.Dir}}{{end}}' ./cmd/gitmoji > "$work/modules"
sort -u "$work/modules" > "$work/sorted"
printf 'Linked Go modules and versions (plus the Go standard library):\n' > "$stage/licenses/MODULES.txt"
tab=$(printf '\t')
while IFS="$tab" read -r module module_version directory; do
  [ -n "$module" ] || continue
  if [ "$target_os" = windows ] && command -v cygpath >/dev/null 2>&1; then
    directory=$(cygpath -u "$directory")
  fi
  notice_dir="$stage/licenses/$module@$module_version"
  mkdir -p "$notice_dir"
  found=false
  for notice in "$directory"/LICENSE* "$directory"/COPYING* "$directory"/NOTICE* "$directory"/PATENTS*; do
    [ -f "$notice" ] || continue
    cp "$notice" "$notice_dir/"
    found=true
  done
  if [ "$found" = false ]; then echo "Missing license notice: $module@$module_version" >&2; exit 1; fi
  printf '%s %s\n' "$module" "$module_version" >> "$stage/licenses/MODULES.txt"
done < "$work/sorted"
printf 'Version: %s\nRevision: %s\nCommit date: %s\nTarget: %s/%s\nToolchain: %s\nSource: %s\n' \
  "$release_version" "$release_revision" "$release_date" "$target_os" "$target_arch" "$(go version)" "$source_state" > "$stage/BUILD.txt"
archive="$name.tar.gz"
tar -czf "$output/$archive" -C "$work" "$name"
if command -v sha256sum >/dev/null 2>&1; then
  (cd "$output" && sha256sum "$archive" > "$archive.sha256")
else
  (cd "$output" && shasum -a 256 "$archive" > "$archive.sha256")
fi
printf 'Created %s and SHA256 checksum.\n' "$output/$archive"
