#!/bin/bash
# Prueba deploy/gremio-update con curl y systemctl simulados. Requiere Linux, root y el usuario de servicio
# (en CI: `sudo useradd --system gremio && sudo deploy/test/gremio-update_test.sh`).
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
updater="$here/../gremio-update"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
export FIXTURES="$work/fixtures" GREMIO_DIR="$work/gremio" GREMIO_READY_TRIES=2 GREMIO_READY_URL=http://127.0.0.1:1/readyz
mkdir -p "$FIXTURES" "$work/bin" "$GREMIO_DIR/data"
fail() { echo "FALLA: $*" >&2; exit 1; }

# Stubs: curl sirve fixtures locales y /readyz falla si el binario instalado dice "broken"; systemctl solo registra.
cat > "$work/bin/curl" <<'STUB'
#!/bin/bash
out=""; url=""
while [ $# -gt 0 ]; do case "$1" in -o) out=$2; shift 2 ;; -*) shift ;; *) url=$1; shift ;; esac; done
case "$url" in
  */readyz) grep -q broken "$GREMIO_DIR/gremio" && exit 22; exit 0 ;;
  */gremio.version) cat "$FIXTURES/gremio.version" ;;
  */releases/latest) printf '{"tag_name": "%s"}' "$(cat "$FIXTURES/latest")" ;;
  */releases/download/*) cp "$FIXTURES/$(basename "$url")" "$out" ;;
  *) exit 22 ;;
esac
STUB
# shellcheck disable=SC2016 # el stub debe escribir $* literal
printf '#!/bin/bash\necho "$*" >> "$FIXTURES/systemctl.log"\n' > "$work/bin/systemctl"
chmod +x "$work/bin/curl" "$work/bin/systemctl"
export PATH="$work/bin:$PATH"

arch=$(uname -m); case "$arch" in aarch64) arch=arm64 ;; x86_64) arch=amd64 ;; esac
release() { # release <tag> <contenido del binario>
  local tag=$1 dir="$work/build-$1"
  mkdir -p "$dir"
  printf '%s\n' "$2" > "$dir/gremio"
  tar -czf "$FIXTURES/gremio_${tag#v}_linux_${arch}.tar.gz" -C "$dir" gremio
  (cd "$FIXTURES" && sha256sum gremio_*.tar.gz > checksums.txt)
}

# 1. Instalación inicial desde la última release.
release v0.1.0 "binario v0.1.0"
echo v0.1.0 > "$FIXTURES/latest"
echo "base v0.1.0" > "$GREMIO_DIR/data/gremio.db"
"$updater" || fail "la instalación inicial falló"
grep -q "binario v0.1.0" "$GREMIO_DIR/gremio" || fail "no instaló v0.1.0"
[ "$(cat "$GREMIO_DIR/.release")" = v0.1.0 ] || fail ".release no quedó en v0.1.0"

# 2. Misma versión: no toca nada.
: > "$FIXTURES/systemctl.log"
"$updater" || fail "la segunda corrida falló"
[ ! -s "$FIXTURES/systemctl.log" ] || fail "reinició el servicio sin versión nueva"

# 3. Versión fijada (GitOps) que no queda lista: rollback del binario y de la base.
release v0.2.0 "binario broken v0.2.0"
echo v0.2.0 > "$FIXTURES/gremio.version"
export GREMIO_VERSION_URL=https://example.org/gremio.version
if "$updater"; then fail "una versión que no queda lista debería devolver error"; fi
grep -q "binario v0.1.0" "$GREMIO_DIR/gremio" || fail "el rollback no restauró el binario"
grep -q "base v0.1.0" "$GREMIO_DIR/data/gremio.db" || fail "el rollback no restauró la base"
[ "$(cat "$GREMIO_DIR/.release")" = v0.1.0 ] || fail ".release cambió tras el rollback"

# 4. Un tag inválido se rechaza antes de descargar nada.
echo 'v1.0.0;rm -rf /' > "$FIXTURES/gremio.version"
if "$updater" 2>/dev/null; then fail "aceptó un tag inválido"; fi

echo "gremio-update: ok"
