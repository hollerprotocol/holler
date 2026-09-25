#!/usr/bin/env bash
# Print the release notes for VERSION: its CHANGELOG.md section, then
# install instructions.
set -euo pipefail

version=${1:?usage: scripts/release-notes.sh VERSION}
version=${version#v}
root=$(cd "$(dirname "$0")/.." && pwd)

awk -v v="$version" '
  index($0, "## [" v "]") == 1 { on = 1; next }
  on && /^## \[/ { exit }
  on { print }
' "$root/CHANGELOG.md"

sed "s/@V@/$version/g" <<'NOTES'

## Install

Pick the archive for your platform (`linux` or `darwin`, `amd64` or `arm64`). The repository is private, so download with `gh`:

```sh
gh release download v@V@ -R hollerprotocol/holler -p 'holler_@V@_linux_amd64.tar.gz'
tar -xzf holler_@V@_linux_amd64.tar.gz holler
install holler ~/.local/bin/          # anywhere on PATH
holler up
```

The agent plugin, with binaries for all four platforms:

```sh
gh release download v@V@ -R hollerprotocol/holler -p 'holler-plugin_@V@.tar.gz'
tar -xzf holler-plugin_@V@.tar.gz     # creates ./holler
claude --plugin-dir ./holler
```

Check downloads against `SHA256SUMS`:

```sh
gh release download v@V@ -R hollerprotocol/holler -p SHA256SUMS
sha256sum -c --ignore-missing SHA256SUMS
```
NOTES
