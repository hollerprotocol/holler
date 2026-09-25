#!/bin/sh
# Install holler: the binary for this machine from a GitHub release, then
# set it up in your agent harnesses (holler bootstrap).
#
#   curl -fsSL https://raw.githubusercontent.com/hollerprotocol/holler/main/install.sh | sh
#
# While the repository is private, fetch the script with gh instead:
#
#   gh api -H 'Accept: application/vnd.github.raw' repos/hollerprotocol/holler/contents/install.sh | sh
#
# Settings (environment variables):
#   HOLLER_VERSION=0.2.0            install this release (default: the latest)
#   HOLLER_INSTALL_DIR=~/bin        where the binary goes (default: ~/.local/bin)
#   HOLLER_BOOTSTRAP=ask|all|none   set up agent harnesses afterwards (default: ask
#                                   when there is a terminal, otherwise print how)
#   GITHUB_TOKEN=...                used for a private repository when gh is not
#                                   installed or not logged in
set -eu

REPO=hollerprotocol/holler
API=https://api.github.com/repos/$REPO

say() { printf '%s\n' "$*"; }
fail() { printf 'holler install: %s\n' "$*" >&2; exit 1; }
have() { command -v "$1" >/dev/null 2>&1; }

# --- where and what ---

case $(uname -s) in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *) fail "unsupported OS $(uname -s): holler builds for Linux and macOS" ;;
esac
case $(uname -m) in
  x86_64 | amd64) arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
  *) fail "unsupported CPU $(uname -m): holler builds for amd64 and arm64" ;;
esac
dir=${HOLLER_INSTALL_DIR:-$HOME/.local/bin}
have tar || fail "tar is required"
if have sha256sum; then
  sha256() { sha256sum "$1" | cut -d' ' -f1; }
elif have shasum; then
  sha256() { shasum -a 256 "$1" | cut -d' ' -f1; }
else
  fail "sha256sum or shasum is required"
fi

# gh when it is installed and logged in (works for a private repository),
# otherwise curl, with GITHUB_TOKEN if one is set.
use_gh=
if have gh && gh auth status >/dev/null 2>&1; then
  use_gh=1
elif ! have curl; then
  fail "curl (or a logged-in gh) is required"
fi

api_get() { # api_get PATH: GET from the GitHub API, with the token if there is one
  if [ -n "${GITHUB_TOKEN:-}" ]; then
    curl -fsSL -H "Authorization: Bearer $GITHUB_TOKEN" -H 'Accept: application/vnd.github+json' "$API$1"
  else
    curl -fsSL -H 'Accept: application/vnd.github+json' "$API$1"
  fi
}

# --- which release ---

if [ -n "${HOLLER_VERSION:-}" ]; then
  tag=v${HOLLER_VERSION#v}
elif [ -n "$use_gh" ]; then
  tag=$(gh release view -R "$REPO" --json tagName -q .tagName) || fail "could not find the latest release"
else
  tag=$(api_get /releases/latest | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n 1)
  [ -n "$tag" ] || fail "could not find the latest release (for a private repository, log in with gh or set GITHUB_TOKEN)"
fi
version=${tag#v}
archive=holler_${version}_${os}_${arch}.tar.gz

# --- download and verify ---

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

# asset_url NAME: the API URL of a release asset, read from release JSON on stdin.
asset_url() {
  tr ',' '\n' | awk -v name="$1" '
    /"url": *"https:\/\/api\.github\.com\/repos\/[^"]*\/releases\/assets\/[0-9]+"/ {
      match($0, /https:[^"]*/); url = substr($0, RSTART, RLENGTH)
    }
    /"name":/ && index($0, "\"" name "\"") { print url; exit }'
}

fetch() { # fetch NAME: download a release asset into $tmp
  if [ -n "$use_gh" ]; then
    gh release download "$tag" -R "$REPO" -p "$1" -D "$tmp" --clobber && return
    fail "could not download $1 from $tag"
  fi
  if curl -fsSL -o "$tmp/$1" "https://github.com/$REPO/releases/download/$tag/$1" 2>/dev/null; then
    return
  fi
  [ -n "${GITHUB_TOKEN:-}" ] || fail "could not download $1 (for a private repository, log in with gh or set GITHUB_TOKEN)"
  [ -n "${release_json:-}" ] || release_json=$(api_get "/releases/tags/$tag") || fail "release $tag not found"
  url=$(printf '%s' "$release_json" | asset_url "$1")
  [ -n "$url" ] || fail "release $tag has no $1"
  curl -fsSL -H "Authorization: Bearer $GITHUB_TOKEN" -H 'Accept: application/octet-stream' -o "$tmp/$1" "$url" ||
    fail "could not download $1"
}

say "Installing holler $version for $os/$arch"
fetch "$archive"
fetch SHA256SUMS
want=$(awk -v f="$archive" '$2 == f || $2 == "*" f { print $1 }' "$tmp/SHA256SUMS")
[ -n "$want" ] || fail "SHA256SUMS has no entry for $archive"
[ "$(sha256 "$tmp/$archive")" = "$want" ] || fail "checksum mismatch for $archive"

tar -xzf "$tmp/$archive" -C "$tmp" holler
mkdir -p "$dir"
# Replace the file rather than overwrite it in place, so a running daemon
# keeps its old binary until it restarts.
cp "$tmp/holler" "$dir/.holler.new"
chmod 0755 "$dir/.holler.new"
mv -f "$dir/.holler.new" "$dir/holler"
say "Installed $("$dir/holler" version) to $dir/holler"

case ":$PATH:" in
  *":$dir:"*) ;;
  *) say "Note: $dir is not on your PATH. Add it, for example:"
     say "  echo 'export PATH=\"$dir:\$PATH\"' >> ~/.profile" ;;
esac

# --- set up agent harnesses ---

if ! "$dir/holler" bootstrap --help >/dev/null 2>&1; then
  exit 0 # releases before 0.2.0 have no bootstrap
fi
case ${HOLLER_BOOTSTRAP:-ask} in
  none)
    say "Run \`holler bootstrap\` to set holler up in your agent harnesses." ;;
  all)
    "$dir/holler" bootstrap --all ;;
  *)
    # When piped into sh, stdin is the script, so ask on the terminal.
    if [ -t 1 ] && (exec </dev/tty) 2>/dev/null; then
      "$dir/holler" bootstrap </dev/tty
    else
      say "Run \`holler bootstrap\` to set holler up in your agent harnesses (Claude Code, Codex, Cursor, ...)."
    fi ;;
esac
