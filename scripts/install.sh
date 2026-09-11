#!/bin/sh
# Install the latest stable GitHub Release without a package manager.
set -eu

main() {
    repository=https://github.com/cross-entropy-ai/git-review
    destination=${INSTALL_DIR:-"$HOME/.local/bin"}

    case "$(uname -s)" in
        Darwin) system=darwin ;;
        Linux) system=linux ;;
        *) echo 'Unsupported OS. Use macOS or Linux.' >&2; exit 1 ;;
    esac
    case "$(uname -m)" in
        x86_64|amd64) arch=amd64 ;;
        arm64|aarch64) arch=arm64 ;;
        *) echo 'Unsupported architecture. Use amd64 or arm64.' >&2; exit 1 ;;
    esac
    for command in curl tar awk install mktemp; do
        command -v "$command" >/dev/null 2>&1 || {
            echo "Required command not found: $command" >&2
            exit 1
        }
    done
    if command -v sha256sum >/dev/null 2>&1; then
        checksum=sha256sum
    elif command -v shasum >/dev/null 2>&1; then
        checksum=shasum
    else
        echo 'SHA-256 verification requires sha256sum or shasum.' >&2
        exit 1
    fi

    # Resolve once, then pin both downloads to the same release during upgrades.
    release=$(curl --proto '=https' --tlsv1.2 -fsSL -o /dev/null -w '%{url_effective}' "$repository/releases/latest")
    tag=${release##*/}
    printf '%s\n' "$tag" | LC_ALL=C grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$' || {
        echo 'Could not find a stable release. Check the GitHub Releases page.' >&2
        exit 1
    }
    archive=git-review_${system}_${arch}.tar.gz
    temporary=$(mktemp -d)
    staged=''
    trap 'rm -rf "$temporary"; if [ -n "$staged" ]; then rm -f "$staged"; fi' EXIT
    trap 'exit 1' HUP INT TERM
    echo "Downloading git-review $tag ($system/$arch)..."
    curl --proto '=https' --tlsv1.2 -fsSL "$repository/releases/download/$tag/$archive" -o "$temporary/$archive"
    curl --proto '=https' --tlsv1.2 -fsSL "$repository/releases/download/$tag/checksums.txt" -o "$temporary/checksums.txt"
    expected=$(awk -v name="$archive" '$2 == name { print $1 }' "$temporary/checksums.txt")
    printf '%s\n' "$expected" | LC_ALL=C grep -Eq '^[0-9a-f]{64}$' || {
        echo 'The release has no valid checksum for this archive.' >&2
        exit 1
    }
    if [ "$checksum" = sha256sum ]; then
        actual=$(sha256sum "$temporary/$archive" | awk '{print $1}')
    else
        actual=$(shasum -a 256 "$temporary/$archive" | awk '{print $1}')
    fi
    [ "$actual" = "$expected" ] || {
        echo 'SHA-256 verification failed; installation stopped.' >&2
        exit 1
    }
    tar -xzf "$temporary/$archive" -C "$temporary" git-review
    mkdir -p "$destination"
    staged=$(mktemp "$destination/.git-review.XXXXXX")
    install -m 755 "$temporary/git-review" "$staged"
    mv -f "$staged" "$destination/git-review"
    staged=''
    echo "Installed git-review $tag to $destination/git-review"
    case ":$PATH:" in
        *":$destination:"*) ;;
        *) printf 'Add this to your shell configuration: export PATH="%s:$PATH"\n' "$destination" ;;
    esac
    echo 'Run git review inside a Git repository. Run this installer again to upgrade.'
}

main "$@"
