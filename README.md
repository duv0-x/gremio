# Gremio

Plataforma open source y self-hosted de perfiles profesionales para gente de tecnología: el LinkedIn descentralizado, compatible con ActivityPub.

Los procesos de selección con tres pruebas técnicas y cuatro entrevistas no escalan para nadie. La propuesta: cada persona tiene **su propia instancia**, publica su perfil y demuestra cómo trabaja con **proyectos reales**.

> **Estado:** fase de decisiones. Todavía no hay código.

## Alcance actual

1. **Perfil**: público para cualquier reclutador o persona en internet, con su panel de configuración.
2. **Proyectos**: escoger, crear y proponer proyectos para estudiar, mejorar habilidades o demostrarlas. Por explorar: modelos de IA que propongan proyectos. La evaluación se estudiará más adelante.

Fase siguiente: armar equipos para proyectos complejos.

## Estructura

- `docs/mockups/`: mockups de referencia (formato `.dc.html` del lienzo de diseño; no son HTML ejecutable).
- `docs/adr/`: decisiones de arquitectura (ADR-001 a ADR-009).
- `cmd/gremio/`: el binario (`serve`, `admin init`, `admin note`, `backup`).
- `internal/store/`: SQLite, migraciones y consultas (sqlc).
- `internal/instance/`: dueño, llaves y host de la instancia.
- `internal/federation/`: WebFinger, host-meta, NodeInfo, Actor, colecciones, inbox, Notes y cola de entrega con reintentos.
- `internal/goap/`: núcleo de go-ap (ActivityPub) mantenido por nosotros.

## Uso

```bash
go build ./cmd/gremio                         # Go 1.26, sin cgo
./gremio admin init --username <usuario>      # crea la base, al dueño y su par de llaves
./gremio serve --host https://<tu-dominio>    # escucha en 127.0.0.1:8080 y federa como @<usuario>@<tu-dominio>
./gremio admin note --text "hola fediverso"   # publica un Note para los seguidores (lo entrega serve)
./gremio backup                               # copia consistente en data/backups/
```

| Flag | Variable | Por defecto |
|------|----------|-------------|
| `--data-dir` | `GREMIO_DATA_DIR` | `./data` |
| `--addr` | `GREMIO_ADDR` | `127.0.0.1:8080` |
| `--host` | `GREMIO_HOST` | obligatorio: URL pública (`https://…`) |
| `--account-domain` | `GREMIO_ACCOUNT_DOMAIN` | el dominio de `--host` |

**`--host` y `--account-domain` son permanentes**: se graban en la base en el primer `serve` y el binario se niega a arrancar con otros valores, porque las demás instancias cachean el Actor y sus llaves (ADR-001). Si `--account-domain` es distinto del dominio de `--host`, ese dominio tiene que redirigir `/.well-known/webfinger`, `host-meta` y `nodeinfo` al host.

## Instalar con systemd

Cada release (`vX.Y.Z`) publica binarios para linux/amd64, linux/arm64 y darwin/arm64 con sus checksums. El `tar.gz` trae también `deploy/` y los avisos de licencia de las dependencias (`third_party_licenses/`).

```bash
sudo useradd --system --home /gremio --shell /usr/sbin/nologin gremio
sudo mkdir -p /gremio/data && sudo chown gremio:gremio /gremio/data && sudo chmod 700 /gremio/data
sudo install -m 0600 deploy/gremio.env.example /gremio/gremio.env   # edita host, account-domain y puerto
sudo install -m 0755 deploy/gremio-update /usr/local/bin/gremio-update
sudo install -m 0644 deploy/gremio.service deploy/gremio-update.service deploy/gremio-update.timer /etc/systemd/system/
sudo systemctl daemon-reload
sudo /usr/local/bin/gremio-update                                   # instala el binario
sudo -u gremio GREMIO_DATA_DIR=/gremio/data /gremio/gremio admin init --username <usuario>
sudo systemctl enable --now gremio gremio-update.timer
```

`gremio-update` instala la última release cada 5 minutos, con verificación del checksum, respaldo del binario y la base, y rollback si `/readyz` no responde. Para fijar la versión estilo GitOps, define `GREMIO_VERSION_URL` en `/gremio/gremio-update.env`. Debe apuntar a un archivo con el tag, p. ej. el `gremio.version` del repo de tu instancia.

## Desarrollo

- `go generate ./...` regenera el código de sqlc (`internal/store`), que se commitea.
- `go vet ./... && go test ./...` antes de cada commit; CI además compila sin cgo para linux/amd64 y linux/arm64 y prueba el actualizador (`deploy/test/gremio-update_test.sh`).
- Releases: pushear un tag `vX.Y.Z` corre GoReleaser (`.github/workflows/release.yml`). Para probar localmente: `go run github.com/goreleaser/goreleaser/v2@v2.19.0 release --snapshot --clean`.

### Probar la federación contra GoToSocial o Mastodon

La instancia tiene que ser alcanzable por HTTPS en un dominio propio: **mastodon.social suspende todo `trycloudflare.com`**, así que un quick tunnel de Cloudflare no sirve con Mastodon. Con un túnel nombrado:

```bash
cloudflared tunnel login                                    # autoriza la zona en el navegador
cloudflared tunnel create gremio-spike
cloudflared tunnel route dns gremio-spike spike.duv0-x.cloud
cloudflared tunnel run --url http://127.0.0.1:8080 gremio-spike
./gremio serve --host https://spike.duv0-x.cloud
```

Después sigue a `@<usuario>@spike.duv0-x.cloud` desde otra instancia. El historial de las pruebas está en gremio#1 y gremio#3.

## Licencia

[AGPL-3.0-or-later](LICENSE). `internal/goap/` es MIT (copiado de go-ap) y la especificación del vocabulario es CC0. Ver [ADR-004](docs/adr/ADR-004-licencia.md) y [CONTRIBUTING.md](CONTRIBUTING.md).
