# internal/goap

Copia propia del núcleo de [GoActivityPub (go-ap)](https://github.com/go-ap). Desde el 2026-10-24 **lo mantenemos nosotros**: no seguimos upstream ni dependemos de sus releases. Ver [ADR-001](../../docs/adr/ADR-001-instancia-y-federacion.md).

## Origen

| Paquete | Upstream | Commit | Fecha del commit |
|---------|----------|--------|------------------|
| `activitypub` (+ `build`, `tests`) | https://github.com/go-ap/activitypub | `73e07f9` | 2026-10-18 |
| `errors` | https://github.com/go-ap/errors | `92e5e4f` | 2026-07-01 |
| `jsonld` | https://github.com/go-ap/jsonld | `737b40e` | 2026-06-07 |
| `xsd` | https://git.sr.ht/~mariusor/go-xsd-duration | `02e7343` | 2022-07-03 |

## Licencia

Todo el código de esta carpeta es **MIT**. Cada paquete conserva su `LICENSE` original con el aviso de copyright, que es la única condición de la licencia: **no borrar esos archivos**.

## Cambios respecto a upstream

- Imports reescritos de `github.com/go-ap/*` y `git.sr.ht/~mariusor/go-xsd-duration` a `github.com/duv0-x/gremio/internal/goap/*`.
- Se quitaron `go.mod`, `go.sum` y `Makefile`: los paquetes son parte del módulo del proyecto.
- `activitypub/decoding_json_test.go`: el test de `JSONGetInt` esperaba `int64`, pero la función devuelve `int` (upstream no compilaba tras `73e07f9`).
- `errors/errors_test.go`: el nombre del archivo de ensamblador ya no está fijado a `asm_amd64.s`, sino que se arma con `runtime.GOARCH` (upstream fallaba en arm64).
- `errors/stack_test.go`: las trazas esperadas usan la nueva ruta del paquete.
- `activitypub/item_collection.go`: se quitó un `return` inalcanzable que reportaba `go vet`.
- `gofmt` aplicado (orden de imports tras el renombre; comentario en `xsd`).

No se copiaron `processing`, `client`, `filters` ni los backends de storage de go-ap.
