#!/bin/bash
# Genera el CSS de la web con el CLI standalone de Tailwind (ADR-008), sin Node.
# La versión y los checksums están fijados; el binario se descarga una vez en .bin/.
# Uso (desde go:generate en internal/web): scripts/tailwind.sh <entrada.css> <salida.css>
set -euo pipefail

VERSION=v4.3.3
case "$(uname -s)-$(uname -m)" in
  Linux-x86_64) platform=linux-x64 sha=dc61b3ac6b8c9ca874c0cc4c57b2409791a64c5540404ca5f5367360babc313a ;;
  Linux-aarch64) platform=linux-arm64 sha=55fd0b241214eff3de1e8ee4f22796662f2d2e7a49bcfca7477cfd0bac398195 ;;
  Darwin-arm64) platform=macos-arm64 sha=cdf646702987a743464dff4d9c60fd4480d1c1e73dd819a9a67f1078815dce9d ;;
  Darwin-x86_64) platform=macos-x64 sha=7922e0953f2110c05976e3bf58f14e643d90427575e766b7d433f5f80cbee7e1 ;;
  *) echo "plataforma no soportada: $(uname -sm)" >&2; exit 1 ;;
esac

root=$(cd "$(dirname "$0")/.." && pwd)
bin="$root/.bin/tailwindcss-$VERSION-$platform"
if [ ! -x "$bin" ]; then
  mkdir -p "$root/.bin"
  tmp=$(mktemp)
  curl -fsSL -o "$tmp" "https://github.com/tailwindlabs/tailwindcss/releases/download/$VERSION/tailwindcss-$platform"
  echo "$sha  $tmp" | shasum -a 256 -c - >/dev/null
  chmod +x "$tmp"
  mv "$tmp" "$bin"
fi
"$bin" --input "$1" --output "$2" --minify 2>/dev/null
