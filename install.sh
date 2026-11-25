#!/usr/bin/env bash
set -u

url="https://github.com/grainologic/papertrail"
tag="latest"
dir=""
dry_run=0

usage() {
  cat <<'EOF'
Usage: install.sh [--url URL] [--tag TAG] [--dir DIR] [--dry-run] [--help]

Downloads a prebuilt pt release binary (no Go required) and places it at
<dir>/pt (pt.exe on Windows).

  --url URL     base repository URL; default: https://github.com/grainologic/papertrail
  --tag TAG     release tag; default: latest
  --dir DIR     install directory; default: ~/.papertrail/bin (%LOCALAPPDATA%\papertrail\bin on Windows)
  --dry-run     print the resolved plan without downloading
  --help        show this help
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --help) usage; exit 0 ;;
    --dry-run) dry_run=1 ;;
    --url) url="${2:-}"; shift ;;
    --tag) tag="${2:-}"; shift ;;
    --dir) dir="${2:-}"; shift ;;
    -*) echo "install.sh: unknown flag: $1" >&2; exit 1 ;;
    *) echo "install.sh: unexpected argument: $1" >&2; exit 1 ;;
  esac
  shift
done

url="${url%/}"
if [ -z "$url" ]; then
  url="https://github.com/grainologic/papertrail"
fi
if [ -z "$tag" ]; then
  tag="latest"
fi

os="$(uname -s)"
case "$os" in
  Linux) os="linux" ;;
  Darwin) os="macos" ;;
  # Git Bash / Cygwin report MINGW/MSYS/CYGWIN yet run the Windows binary.
  MINGW*|MSYS*|CYGWIN*) os="windows" ;;
  *) echo "install.sh: unsupported OS: $os" >&2; exit 1 ;;
esac

arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *) echo "install.sh: unsupported architecture: $arch" >&2; exit 1 ;;
esac

if [ "$os" = "windows" ]; then
  ext="zip"
  bin="pt.exe"
  if [ -z "$dir" ]; then
    if [ -n "${LOCALAPPDATA:-}" ]; then
      if command -v cygpath >/dev/null 2>&1; then
        dir="$(cygpath -u "$LOCALAPPDATA")/papertrail/bin"
      else
        dir="${LOCALAPPDATA}/papertrail/bin"
      fi
    else
      dir="${HOME}/AppData/Local/papertrail/bin"
    fi
  fi
else
  ext="tar.gz"
  bin="pt"
  if [ -z "$dir" ]; then
    dir="${HOME}/.papertrail/bin"
  fi
fi

if [ "$dir" != "/" ]; then
  dir="${dir%/}"
fi

asset="pt_${os}_${arch}.${ext}"
if [ "$tag" = "latest" ]; then
  base_url="${url}/releases/latest/download"
else
  base_url="${url}/releases/download/${tag}"
fi
download_url="${base_url}/${asset}"
checksum_url="${base_url}/checksums.txt"

echo "url:      ${url}"
echo "tag:      ${tag}"
echo "os/arch:  ${os}/${arch}"
echo "asset:    ${asset}"
echo "download: ${download_url}"
echo "checksum: ${checksum_url}"
echo "dest:     ${dir}/${bin}"

if [ "$dry_run" = "1" ]; then
  exit 0
fi

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

curl -fsSL -o "$tmpdir/${asset}" "$download_url" || {
  echo "install.sh: failed to download ${download_url}" >&2
  exit 1
}
curl -fsSL -o "$tmpdir/checksums.txt" "$checksum_url" || {
  echo "install.sh: failed to download ${checksum_url}" >&2
  exit 1
}

expected="$(awk -v a="$asset" '$2 == a { print $1 }' "$tmpdir/checksums.txt")"
if [ -z "$expected" ]; then
  echo "install.sh: no checksum entry for ${asset}" >&2
  exit 1
fi
actual="$(sha256sum "$tmpdir/${asset}" | awk '{ print $1 }')"
if [ "$actual" != "$expected" ]; then
  echo "install.sh: checksum mismatch for ${asset}" >&2
  exit 1
fi

mkdir -p "$dir"
if [ "$ext" = "zip" ]; then
  unzip -q -o "$tmpdir/${asset}" -d "$tmpdir/extract"
else
  mkdir -p "$tmpdir/extract"
  tar -xzf "$tmpdir/${asset}" -C "$tmpdir/extract"
fi
cp "$tmpdir/extract/${bin}" "${dir}/${bin}"
if [ "$os" != "windows" ]; then
  chmod +x "${dir}/${bin}"
fi
echo "installed ${dir}/${bin}"

add_to_path() {
  case ":${PATH}:" in
    *":${dir}:"*) return 0 ;;
  esac

  if [ "$os" = "windows" ] && command -v powershell.exe >/dev/null 2>&1 && command -v cygpath >/dev/null 2>&1; then
    win_dir="$(cygpath -w "$dir")"
    if added="$(PT_ADD_PATH_DIR="$win_dir" powershell.exe -NoProfile -Command '
      $entry = $env:PT_ADD_PATH_DIR
      $norm = $entry.TrimEnd("\")
      $cur = [Environment]::GetEnvironmentVariable("Path", "User")
      $parts = @()
      if ($cur) { $parts = $cur -split ";" | Where-Object { $_ -ne "" } }
      if (-not ($parts | Where-Object { $_.TrimEnd("\") -ieq $norm })) {
        $next = if ($parts.Count -gt 0) { ($parts + $entry) -join ";" } else { $entry }
        [Environment]::SetEnvironmentVariable("Path", $next, "User")
        Write-Output "ADDED"
      }
    ' 2>/dev/null)"; then
      case "$added" in
        *ADDED*) echo "added ${win_dir} to user PATH (restart open shells to pick it up)" ;;
      esac
      return 0
    fi
  fi

  shell_name="$(basename "${SHELL:-}")"
  case "$shell_name" in
    zsh) profile="${ZDOTDIR:-$HOME}/.zshrc"; line="export PATH=\"${dir}:\$PATH\"" ;;
    fish) profile="${XDG_CONFIG_HOME:-$HOME/.config}/fish/config.fish"; line="fish_add_path -m \"${dir}\"" ;;
    bash)
      if [ -f "${HOME}/.bashrc" ]; then
        profile="${HOME}/.bashrc"
      elif [ -f "${HOME}/.bash_profile" ]; then
        profile="${HOME}/.bash_profile"
      else
        profile="${HOME}/.profile"
      fi
      line="export PATH=\"${dir}:\$PATH\""
      ;;
    *) profile="${HOME}/.profile"; line="export PATH=\"${dir}:\$PATH\"" ;;
  esac

  if [ -f "$profile" ] && grep -Fq "$dir" "$profile" 2>/dev/null; then
    return 0
  fi
  mkdir -p "$(dirname "$profile")"
  printf '\n%s\n' "$line" >> "$profile"
  echo "added ${dir} to PATH in ${profile} (restart your shell or run: export PATH=\"${dir}:\$PATH\")"
}

add_to_path
