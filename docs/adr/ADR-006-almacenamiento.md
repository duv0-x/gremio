---
title: "Almacenamiento: SQLite con ncruces, sqlc y goose"
number: 6
date: 2026-10-24
domain: architecture
status: "aceptado"
tags: [storage, sqlite, ncruces, sqlc, goose, migrations, backup]
---

# ADR-006: Almacenamiento: SQLite con ncruces, sqlc y goose

## Contexto

Cada instancia es de una sola persona ([ADR-001](ADR-001-instancia-y-federacion.md)) y se distribuye como un binario único. Hay que guardar el perfil y sus vistas ([ADR-003](ADR-003-perfil-v1.md)), los proyectos con sus versiones, los snapshots y las ejecuciones ([ADR-002](ADR-002-proyecto-y-ejecucion.md)), y el estado de federación (seguidores, cola de entrega, llaves). La arquitectura tentativa decía "SQLite por defecto, Postgres opcional", como GoToSocial.

## Decisión

### 1. Solo SQLite

- Gremio soporta **un solo motor: SQLite**. La carga de una persona no justifica un servidor de base de datos, y un solo dialecto significa una sola batería de tests y migraciones.
- La base es un archivo en el directorio de datos configurable de la instancia (p. ej. `/data/gremio.db`). En Kubernetes va en un PVC.
- Configuración al abrir: `journal_mode=WAL`, `foreign_keys=ON`, `busy_timeout`, `synchronous=NORMAL`.
- **Un solo escritor**: un pool de una conexión para escribir y un pool aparte para leer. Así se evitan los `SQLITE_BUSY` bajo escritura concurrente (inbox y cola de entrega a la vez).

### 2. Driver: `github.com/ncruces/go-sqlite3`

- SQLite oficial compilado a WASM y ejecutado con `wazero`, a través de `database/sql` (`github.com/ncruces/go-sqlite3/driver`). Go puro: compilación cruzada a linux/arm64 sin toolchain de C.
- Es el driver por defecto de GoToSocial. La alternativa `modernc.org/sqlite`, en cambio, GoToSocial la reemplaza por un fork parcheado para un bug de concurrencia.
- La compilación del WASM al arrancar se cachea en disco (como en la instancia de GoToSocial de la Pi).

### 3. Perfil como documento, el resto en tablas

- El **perfil** (JSON Resume + `id` por elemento + `evidence`) se guarda como **un documento JSON** en una tabla de una sola fila. Se valida contra un struct de Go en cada escritura, y SQLite ofrece funciones `json_*` si hace falta consultar dentro.
- Todo lo que tiene identidad propia o se consulta por separado va en **tablas normales**. Tablas iniciales (orientativas):

| Tabla | Contenido |
|-------|-----------|
| `profile` | El documento del perfil (una fila) |
| `role_views` | Vistas por rol: slug, resumen y referencias a `id` del perfil |
| `projects` / `project_versions` | Proyectos propios y sus versiones inmutables |
| `project_snapshots` | Enunciados copiados de otras instancias (`basedOn` + versión) |
| `executions` / `execution_links` | Ejecuciones, su estado, visibilidad y enlaces |
| `followers` | Seguidores remotos (Actor, inbox, `sharedInbox`) |
| `delivery_queue` | Entregas salientes pendientes, intentos y próximo reintento |
| `instance_keys` | Par de llaves del Actor para las HTTP Signatures |

### 4. Acceso: sqlc; migraciones: goose

- Las consultas se escriben en SQL (`internal/db/queries/*.sql`) y **sqlc** genera código Go tipado sobre `database/sql`. El código generado se commitea. No hay ORM.
- Las migraciones son archivos SQL de **goose** embebidos en el binario (`embed.FS`) y se aplican al arrancar, antes de servir tráfico.
- Las migraciones solo avanzan en producción: no se ejecutan `down`. Si una migración sale mal, se restaura el respaldo previo.
- Mientras corren las migraciones, los comandos de administración esperan o fallan con un mensaje claro. Es la lección de GoToSocial en la Pi: `admin account create` hacía panic si se corría durante la primera migración.

### 5. Respaldos

- `gremio backup` genera una copia consistente con `VACUUM INTO`, sin detener el servidor.
- Antes de aplicar migraciones, el binario hace un respaldo automático en el directorio de datos.
- Litestream (réplica continua a S3/GCS) queda como opción documentada, no integrada.

## Consecuencias positivas

- "Un binario + un archivo": desplegar, respaldar y mover una instancia es copiar dos cosas.
- Un solo dialecto: tests, migraciones y consultas no se duplican.
- sqlc verifica el SQL contra el esquema al generar: un error de consulta aparece antes de compilar, no en producción.
- Go puro de punta a punta: compilación cruzada simple para la Pi y para contenedores.

## Consecuencias negativas

- Si algún día hiciera falta una instancia multiusuario grande, habría que reabrir esta decisión (y ADR-001).
- ncruces agrega `wazero` al binario y un costo de compilación en el primer arranque.
- El perfil como documento implica que cambiar su forma requiere migrar JSON en Go, no solo `ALTER TABLE`.
- sqlc es una herramienta más en el flujo de desarrollo (hay que regenerar al cambiar consultas o esquema).

## Alternativas consideradas

| Alternativa | Razón de descarte |
|-------------|-------------------|
| SQLite + Postgres opcional | Duplica dialectos y tests para un caso que una instancia de una persona no necesita |
| Solo Postgres | Obliga a correr un servidor de base de datos junto a cada instancia |
| `modernc.org/sqlite` | GoToSocial necesitó un fork parcheado para un bug de concurrencia |
| `mattn/go-sqlite3` | Requiere cgo: compilación cruzada con toolchain de C |
| Perfil totalmente normalizado | Mucho esquema para un documento que se lee y escribe casi siempre entero |
| bun (ORM) | Tiene sentido sobre todo con varios motores; agrega abstracción sobre el SQL |
| `database/sql` + SQL a mano | Mucho código repetitivo de escaneo; sin verificación de las consultas |

## Pendiente de resolver

- Cómo se fija la versión de sqlc en el repo (`go tool` o binario fijado en CI).
- Política de retención de respaldos automáticos.

## Links relacionados

- [ADR-001](ADR-001-instancia-y-federacion.md): cola de entrega y llaves.
- [ADR-002](ADR-002-proyecto-y-ejecucion.md): proyectos, versiones, snapshots y ejecuciones.
- [ADR-003](ADR-003-perfil-v1.md): el documento del perfil y las vistas.
- Referencia: `go.mod` de `/Users/lx-duv0-x/Documents/Repositories/echo.duv0-x.cloud` (ncruces, bun, pgx).
