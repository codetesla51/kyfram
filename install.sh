#!/bin/sh
set -e
# kyfram installer — checks ffmpeg, downloads the prebuilt binary.
# Nothing is auto-installed; missing ffmpeg is reported with a hint.
# usage:
#   curl -fsSL https://kyfram-site-68056d.gitlab.io/install.sh | sh
#   curl -fsSL https://kyfram-site-68056d.gitlab.io/install.sh | sh -s -- v0.3.0
#   VERSION=v0.3.0 sh install.sh
REPO="codetesla51/kyfram"
VERSION="${VERSION:-${1:-latest}}"

printf 'kyfram installer\n\n'

# first suitable package manager, for the ffmpeg hint only
pm=""
for p in pacman apt-get dnf apk zypper brew; do
  command -v "$p" >/dev/null 2>&1 && { pm="$p"; break; }
done
pminstall() {
  case "$pm" in
    pacman)  echo "sudo pacman -S $1" ;;
    apt-get) echo "sudo apt-get install -y $1" ;;
    dnf)     echo "sudo dnf install -y $1" ;;
    apk)     echo "sudo apk add $1" ;;
    zypper)  echo "sudo zypper install $1" ;;
    brew)    echo "brew install $1" ;;
    *)       echo "install $1 with your package manager" ;;
  esac
}

# ffmpeg — the only runtime dep (mp4 encoding; png renders work without it)
if v=$(ffmpeg -version 2>/dev/null | sed -n 1p 2>/dev/null); then
  set -- $v
  printf '  [ok]  %-9s %s\n' ffmpeg "${3:-installed}"
else
  printf '  [no]  %-9s %s\n' ffmpeg "not found — $(pminstall ffmpeg)"
  printf '\nkyfram needs ffmpeg to encode mp4.\n'
  printf 'Install it (hint above), then re-run:\n'
  printf '  curl -fsSL https://kyfram-site-68056d.gitlab.io/install.sh | sh\n'
  exit 1
fi

# resolve version: latest -> newest release tag
case "$VERSION" in
  latest)
    api="https://gitlab.com/api/v4/projects/codetesla51%2Fkyfram/releases/permalink/latest"
    TAG=$(curl -fsSL "$api" 2>/dev/null | sed -n 's/.*"tag_name":"\([^"]*\)".*/\1/p' 2>/dev/null)
    [ -z "$TAG" ] && TAG="v0.3.0" # ponytail: offline fallback pin, bump when a release lands
    ;;
  v*) TAG="$VERSION" ;;
  *)  TAG="v$VERSION" ;;
esac

case "$(uname -s)-$(uname -m)" in
  Linux-x86_64)  osarch="linux-amd64" ;;
  Linux-aarch64) osarch="linux-arm64" ;;
  Darwin-x86_64) osarch="darwin-amd64" ;;
  Darwin-arm64)  osarch="darwin-arm64" ;;
  *)
    printf '\nno prebuilt binary for $(uname -s)-$(uname -m) — build from source instead:\n'
    printf '  go install gitlab.com/codetesla51/kyfram/cmd/kyfram@%s\n' "$TAG"
    exit 1
    ;;
esac

bin_dir=/usr/local/bin
if [ ! -w "$bin_dir" ]; then
  bin_dir="$HOME/.local/bin"
  mkdir -p "$bin_dir"
fi

echo
printf '  fetching  %s (%s)\n' kyfram "$TAG"
url="https://gitlab.com/$REPO/-/jobs/artifacts/$TAG/raw/dist/kyfram-$osarch?job=build-binaries"
# loader: progress bar on terminals, silent in pipes
[ -t 2 ] && bar="-#" || bar="-sS"
if ! curl $bar -fL -o "$bin_dir/kyfram" "$url"; then
  rm -f "$bin_dir/kyfram"
  printf 'download failed — release %s has no prebuilt binaries yet (they are published on new tags).\n' "$TAG" >&2
  printf 'try again after the next release, or build from source:\n' >&2
  printf '  go install gitlab.com/codetesla51/kyfram/cmd/kyfram@%s\n' "$TAG" >&2
  exit 1
fi
chmod +x "$bin_dir/kyfram"

out=$("$bin_dir/kyfram" --version 2>/dev/null || echo kyfram)
printf '  installed %s (%s)\n' "$bin_dir/kyfram" "$out"

case ":$PATH:" in
  *":$bin_dir:"*) ;;
  *) printf '\nNOTE: %s is not on your PATH — add it:\n  export PATH="$PATH:%s"\n' "$bin_dir" "$bin_dir" ;;
esac

printf '\nnext steps:\n'
printf '  kyfram new demo\n'
printf '  kyfram render --out demo/out/demo.mp4 demo/scene.lgv\n'