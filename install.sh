#!/usr/bin/env sh
set -e

# detect OS
detect_os() {
    case "$(uname -s)" in
        Linux*)     echo "linux";;
        Darwin*)    echo "darwin";;
        FreeBSD*)   echo "freebsd";;
        MINGW*|MSYS*|CYGWIN*) echo "windows";;
        *)
            echo "error: unsupported OS $(uname -s)" >&2
            exit 1
            ;;
    esac
}

# detect architecture
detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64)   echo "amd64";;
        aarch64|arm64)  echo "arm64";;
        riscv64)        echo "riscv64";;
        *)
            echo "error: unsupported architecture $(uname -m)" >&2
            exit 1
            ;;
    esac
}

# set install directory
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

# detect platform
OS=$(detect_os)
ARCH=$(detect_arch)

echo "installing nak ($OS-$ARCH) to $INSTALL_DIR..."

# check if curl is available
command -v curl >/dev/null 2>&1 || { echo "error: curl is required" >&2; exit 1; }

# get latest release tag
RELEASE_INFO=$(curl -s https://api.github.com/repos/fiatjaf/nak/releases/latest)
TAG="${RELEASE_INFO#*\"tag_name\"}"
TAG="${TAG#*\"}"
TAG="${TAG%%\"*}"

[ -z "$TAG" ] && { echo "error: failed to fetch release info" >&2; exit 1; }

# construct download URL
BINARY_NAME="nak-${TAG}-${OS}-${ARCH}"
[ "$OS" = "windows" ] && BINARY_NAME="${BINARY_NAME}.exe"
DOWNLOAD_URL="https://github.com/fiatjaf/nak/releases/download/${TAG}/${BINARY_NAME}"

# create install directory and download
mkdir -p "$INSTALL_DIR"
TARGET_PATH="$INSTALL_DIR/nak"
[ "$OS" = "windows" ] && TARGET_PATH="${TARGET_PATH}.exe"

if curl -sS -L -f -o "$TARGET_PATH" "$DOWNLOAD_URL"; then
    chmod +x "$TARGET_PATH"
    echo "installed nak $TAG to $TARGET_PATH"

    # check if install dir is in PATH
    case ":$PATH:" in
        *":$INSTALL_DIR:"*) ;;
        *) echo "note: add $INSTALL_DIR to your PATH" ;;
    esac
else
    echo "error: download failed from $DOWNLOAD_URL" >&2
    exit 1
fi
