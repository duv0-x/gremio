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
- `docs/adr/`: decisiones de arquitectura (ADR-001 a ADR-008).
- `internal/goap/`: núcleo de go-ap (ActivityPub) mantenido por nosotros.

## Uso

```bash
go build ./cmd/gremio                         # Go 1.26, sin cgo
./gremio admin init --username <usuario>      # crea la base, al dueño y su par de llaves
./gremio serve                                # escucha en 127.0.0.1:8080 (GET /readyz)
./gremio backup                               # copia consistente en data/backups/
```

El directorio de datos se elige con `--data-dir` o `GREMIO_DATA_DIR` (por defecto `./data`), y la dirección con `--addr` o `GREMIO_ADDR`.

## Desarrollo

- `go generate ./...` regenera el código de sqlc (`internal/store`), que se commitea.
- `go vet ./... && go test ./...` antes de cada commit; CI además compila sin cgo para linux/amd64 y linux/arm64.

## Licencia

[AGPL-3.0-or-later](LICENSE). `internal/goap/` es MIT (copiado de go-ap) y la especificación del vocabulario es CC0. Ver [ADR-004](docs/adr/ADR-004-licencia.md) y [CONTRIBUTING.md](CONTRIBUTING.md).
