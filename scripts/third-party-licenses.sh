#!/bin/bash
# Genera third_party_licenses/ con los avisos de licencia de todo lo compilado en el binario:
# MIT, BSD y Apache exigen incluirlos también en las distribuciones binarias. Lo corre GoReleaser.
set -euo pipefail

out=${1:-third_party_licenses}
chmod -R u+w "$out" 2>/dev/null || true # go-licenses deja los archivos de solo lectura
rm -rf "$out"

# go-licenses no reconoce MIT-0: ese módulo se copia a mano más abajo.
go run github.com/google/go-licenses/v2@v2.0.1 save ./cmd/gremio --save_path="$out" \
  --ignore github.com/duv0-x/gremio \
  --ignore github.com/ncruces/go-sqlite3-wasm/v6

wasm=$(go list -m -f '{{.Dir}}' github.com/ncruces/go-sqlite3-wasm/v6)
mkdir -p "$out/github.com/ncruces/go-sqlite3-wasm/v6"
cp "$wasm/LICENSE" "$out/github.com/ncruces/go-sqlite3-wasm/v6/" # MIT-0; el SQLite que incluye es de dominio público

# Copia propia de go-ap (MIT) dentro del módulo de gremio; go-licenses la considera código nuestro.
for d in internal/goap/*/; do
  name=$(basename "$d")
  mkdir -p "$out/internal/goap/$name"
  cp "$d/LICENSE" "$out/internal/goap/$name/"
done
