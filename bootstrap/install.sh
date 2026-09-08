#!/bin/sh
# shanty bootstrap — fetch the right binary for this machine and put it in
# ~/.local/bin. This is the only shell in the project and it stays small enough
# to read before running.
#
#   curl -fsSL https://raw.githubusercontent.com/bspeelm/shanty/main/bootstrap/install.sh | sh
#
# It installs shanty and nothing else. shanty does not decode audio itself, so
# mpv has to be there too (ADR-011); this says so rather than leaving you with
# a binary that starts and cannot play.
set -eu

REPO="${SHANTY_REPO:-bspeelm/shanty}"
BASE="${SHANTY_BASE_URL:-https://github.com/$REPO/releases}"
VERSION="${SHANTY_VERSION:-latest}"
BINDIR="${SHANTY_BINDIR:-$HOME/.local/bin}"

for a in "$@"; do
    case "$a" in --verify) SHANTY_VERIFY=1 ;; esac
done

case "$(uname -s)" in
    Linux)  os=linux ;;
    Darwin) os=darwin ;;
    *) echo "shanty: unsupported system $(uname -s)" >&2; exit 1 ;;
esac

case "$(uname -m)" in
    x86_64|amd64)  arch=amd64 ;;
    aarch64|arm64) arch=arm64 ;;
    *) echo "shanty: unsupported architecture $(uname -m)" >&2; exit 1 ;;
esac

if [ "$VERSION" = latest ]; then
    base="$BASE/latest/download"
else
    base="$BASE/download/$VERSION"
fi
archive="shanty_${os}_${arch}.tar.gz"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# fetch <url> <destination>. Either curl or wget, both told to fail loudly on
# an HTTP error rather than writing the error page into the file.
fetch() {
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL "$1" -o "$2"
    elif command -v wget >/dev/null 2>&1; then
        wget -qO "$2" "$1"
    else
        echo "shanty: need curl or wget" >&2
        exit 1
    fi
}

echo "shanty: downloading $os/$arch"
fetch "$base/$archive" "$tmp/$archive"

# The checksum catches corruption and truncation. It does not catch a
# compromised release, because whoever could swap the archive could swap
# checksums.txt beside it -- that is what --verify is for.
if command -v sha256sum >/dev/null 2>&1; then
    sha_cmd="sha256sum"
elif command -v shasum >/dev/null 2>&1; then
    sha_cmd="shasum -a 256"
fi

if [ -n "${sha_cmd:-}" ] && fetch "$base/checksums.txt" "$tmp/checksums.txt" 2>/dev/null; then
    want="$(awk -v f="$archive" '$2 == f || $2 == "*" f { print $1 }' "$tmp/checksums.txt")"
    if [ -z "$want" ]; then
        echo "shanty: $archive is not listed in checksums.txt" >&2
        exit 1
    fi
    got="$($sha_cmd "$tmp/$archive" | cut -d" " -f1)"
    if [ "$want" != "$got" ]; then
        echo "shanty: checksum mismatch for $archive" >&2
        echo "        expected $want" >&2
        echo "        got      $got" >&2
        exit 1
    fi
    echo "shanty: checksum verified"
else
    # Say what did not happen rather than implying a check that did.
    echo "shanty: no checksum available; skipping verification" >&2
fi

# Provenance, on request. The checksum proves the bytes match what the release
# published; only this proves who published them. Off by default because it
# needs the gh CLI, and an installer that fails merely because gh is absent is
# worse than one that says what it did not check.
if [ -n "${SHANTY_VERIFY:-}" ]; then
    if ! command -v gh >/dev/null 2>&1; then
        echo "shanty: asked to verify provenance, but the gh CLI is not installed" >&2
        echo "        install it, or drop --verify to install with the checksum alone" >&2
        exit 1
    fi
    if ! fetch "$base/attestation.jsonl" "$tmp/attestation.jsonl" 2>/dev/null; then
        echo "shanty: asked to verify provenance, but this release publishes none" >&2
        exit 1
    fi
    if ! gh attestation verify "$tmp/$archive" --repo "$REPO" \
            --bundle "$tmp/attestation.jsonl" >/dev/null 2>&1; then
        echo "shanty: provenance verification FAILED for $archive" >&2
        echo "        these bytes do not carry a signature from $REPO's release workflow" >&2
        exit 1
    fi
    echo "shanty: provenance verified -- built by $REPO in GitHub Actions"
else
    echo "shanty: run with --verify to check provenance too (needs the gh CLI)"
fi

tar -xzf "$tmp/$archive" -C "$tmp"
mkdir -p "$BINDIR"
install -m 755 "$tmp/shanty" "$BINDIR/shanty"

echo "shanty: installed to $BINDIR/shanty"

# Shell completion is a file each shell reads from a directory of its own, so
# it is put there rather than added to a startup file. A failure here is not
# worth failing an install over.
"$BINDIR/shanty" completions install >/dev/null 2>&1 &&
    echo "shanty: shell completion installed"

# ~/.local/bin missing from PATH is the commonest reason a fresh install looks
# like it did nothing, so say so now rather than letting the next command fail
# with "not found".
case ":$PATH:" in
    *":$BINDIR:"*) ;;
    *) echo
       echo "shanty: $BINDIR is not on your PATH. Add it:"
       echo "        export PATH=\"\$HOME/.local/bin:\$PATH\"" ;;
esac

# shanty plays through mpv and does not decode anything itself. A binary that
# installs cleanly and then cannot play is a worse first minute than being told
# now.
if ! command -v mpv >/dev/null 2>&1; then
    echo
    echo "shanty: mpv is not installed, and shanty plays through it."
    echo "        apt install mpv · dnf install mpv · brew install mpv · pacman -S mpv"
fi

echo
echo "next: shanty doctor"
