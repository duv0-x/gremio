---
title: "Modelo de instancia y alcance de la federación ActivityPub"
number: 1
date: 2026-10-24
domain: architecture
status: "aceptado"
tags: [activitypub, federation, instance, go-ap, single-user]
---

# ADR-001: Modelo de instancia y alcance de la federación ActivityPub

## Contexto

La plataforma es un "LinkedIn descentralizado": cada persona tiene su propia instancia, en su dominio, y se conecta con el resto del fediverso por ActivityPub. Hay que decidir cuántas cuentas soporta una instancia, qué ve un seguidor desde Mastodon o GoToSocial, qué interacciones entrantes procesamos y con qué librería se implementa ActivityPub en Go.

El objetivo es una plataforma descentralizada y robusta, así que se prefiere una base probada en producción a una implementación propia mínima.

## Decisión

### 1. Una instancia = una cuenta

- El binario tiene exactamente un dueño, creado por CLI al instalar. No hay registro público, roles ni moderación de usuarios.
- Hospedar a varias personas = un contenedor (o proceso) por persona.

### 2. Identidad

- Handle `@usuario@dominio`, resuelto por WebFinger (`/.well-known/webfinger`), con `host-meta` y `nodeinfo`.
- Se soporta separar `host` (donde corre el servidor) de `account-domain` (el dominio del handle), como hace GoToSocial: `@duvan@duv0-x.cloud` servido desde un subdominio.
- **`host`, `account-domain` y usuario son permanentes** una vez que la instancia federa: las demás instancias cachean el Actor y sus llaves. La CLI de instalación lo advierte.

### 3. Qué se federa (v1): perfil + actividad

- **Perfil** como Actor `Person`: `preferredUsername`, `name`, `summary`, `icon`, `url` (la página HTML del perfil), `attachment` con `PropertyValue` para los links (verificables con `rel="me"`), `publicKey`, `inbox`, `outbox`, `followers`.
- Cada cambio de perfil se envía como `Update` del `Person` a los seguidores.
- **Actividad de proyectos** como `Create` de un `Note` público con enlace a la página del proyecto en la instancia ("empezó / terminó el proyecto X"). Mastodon y GoToSocial lo muestran en el timeline sin extensiones.
- Los proyectos **no** se federan todavía como objetos ricos (requisitos, DoD); eso es materia del ADR del modelo Proyecto/Ejecución.

### 4. Inbox (v1): solo follows

- Se procesan `Follow` (aceptado automáticamente con `Accept`; `manuallyApprovesFollowers` configurable), `Undo{Follow}` y `Delete` del Actor remoto (se borra el seguidor).
- Todo lo demás (`Like`, `Announce`, replies, `Create` entrantes) se responde con `202 Accepted` y se descarta sin persistir.
- Todo POST al inbox exige una HTTP Signature válida y un `Digest` que coincida con el cuerpo; el Actor de la actividad debe ser el dueño de la llave que firma.

### 5. Implementación

- **Copia propia del núcleo de go-ap** (`activitypub`, `errors`, `jsonld` y `go-xsd-duration`) en `internal/goap/`, para tipos ActivityStreams y serialización. **La mantenemos nosotros**: no seguimos upstream ni dependemos de sus releases. Licencia MIT: se conservan los `LICENSE` originales y no condiciona la licencia del proyecto. Origen, commits y cambios: `internal/goap/README.md`.
- La lógica del protocolo (inbox, outbox, entrega) se escribe en el proyecto; no se copian `processing` ni `client` de go-ap.
- **`code.superseriousbusiness.org/httpsig`** (BSD-3, el que usa GoToSocial contra Mastodon) para firmar y verificar las HTTP Signatures, como dependencia normal.
- El envío a los seguidores (fan-out a `sharedInbox` cuando exista) se hace con reintentos, a través de una cola persistida en la base de datos. Así una caída no pierde entregas.
  - **Política (gremio#7):** toda actividad saliente, incluido el `Accept`, pasa por la tabla `delivery_queue` y se firma de nuevo en cada intento. Los reintentos usan backoff exponencial desde 30 s (30 s, 1 min, 2 min… hasta 6 h). Tras 12 intentos, unas 14,5 h, la entrega se descarta.
  - Un 4xx (salvo 408 y 429) se descarta al primer intento; los 5xx, los timeouts y los errores de red se reintentan.
  - Un `Note` se encola una sola vez por servidor: el `sharedInbox` si el seguidor lo publica.
  - La cola la vacía un worker dentro de `gremio serve`, que la revisa cada 5 s o al instante cuando el mismo proceso encola. `gremio admin note` solo escribe en la base.
- **Todos los GETs salientes a otras instancias van firmados** (`(request-target)`, `host`, `date`): GoToSocial firma los suyos y exige firma para entregar un Actor.
- La entrega usa el `sharedInbox` si el Actor remoto lo publica (Mastodon sí) y, si no, su inbox personal (GoToSocial no publica `sharedInbox`).
- El dueño de una llave se toma de `publicKey.owner`: GoToSocial usa `keyId` con ruta (`/users/x/main-key`) y Mastodon con fragmento (`#main-key`).
- Las pruebas de interoperabilidad se hacen contra `echo.duv0-x.cloud` (GoToSocial propio) y contra una cuenta de Mastodon.

## Validación: spike gremio#1 (2026-10-24)

Una instancia mínima (`spike/federation/`, retirada en gremio#6 cuando su código pasó a `internal/federation/`; estado en memoria, expuesta con un quick tunnel de Cloudflare) federó con `echo.duv0-x.cloud` (GoToSocial 0.22.2) usando `internal/goap` y `code.superseriousbusiness.org/httpsig`:

- `@root@duv0-x.cloud` encontró al Actor por WebFinger y lo siguió; el spike verificó firma y `Digest` y GoToSocial aceptó el `Accept` firmado.
- Un `Create{Note}` firmado apareció en el timeline de inicio de `@root`.
- El `Undo{Follow}` eliminó al seguidor.
- Antes de seguir, GoToSocial recorre el Actor completo con GETs firmados: Actor, `followers`, `following` y `outbox`. Las tres colecciones deben existir aunque estén vacías.
- GoToSocial acepta dominios `*.trycloudflare.com`.

Y contra `mastodon.social` (Mastodon 5.0.0-nightly, gremio#3), con `@duv0_x@mastodon.social`:

- Follow aceptado, `Note` en el timeline de inicio y `Undo{Follow}` procesado, sin cambiar el código del spike.
- Mastodon también firma sus GETs y recorre Actor, `outbox`, `following` y `followers` antes de seguir, y otra vez después del `Accept`.
- Mastodon **repite el WebFinger después de obtener el Actor**: verifica que el dominio del handle corresponda al Actor. Con `account-domain` distinto del `host`, el WebFinger del dominio del handle tiene que apuntar al Actor.
- **Con `account-domain` distinto del `host`, el WebFinger tiene que responder también `acct:usuario@<host>`** con el `subject` canónico `acct:usuario@<account-domain>`. Mastodon verifica el Actor con ese recurso, y si recibe un 404 lo descarta sin mostrar nada. Se detectó en producción tras la migración a `work.duv0-x.cloud` (gremio#17).
- Mastodon publica `sharedInbox` (`https://mastodon.social/inbox`): los `Note` van ahí.
- **mastodon.social suspende todo `trycloudflare.com`** (motivo "Spam" en su lista pública de bloqueos): una instancia de Gremio necesita dominio propio. Las pruebas se hicieron con un túnel nombrado en `spike.duv0-x.cloud`.
- Las pruebas fueron contra una instancia sin `AUTHORIZED_FETCH` confirmado; como el spike firma todos sus GETs, debería funcionar también con instancias que lo exigen, pero no se probó.

## Consecuencias positivas

- Un modelo de un solo usuario elimina registro, moderación y multi-tenant: menos superficie de ataque y de código.
- Con perfil + `Note` se tiene presencia real en el fediverso (seguir, ver actividad) usando solo tipos que todos entienden.
- go-ap es modular, idiomático y lo usa Forgejo en producción; al ser una copia propia, ningún cambio upstream nos rompe y lo adaptamos libremente.
- Inbox restringido a follows: casi nada de spam ni contenido de terceros que moderar.

## Consecuencias negativas

- Hospedar a muchas personas cuesta un proceso por persona; una instancia comunitaria requeriría otro ADR.
- No hay comentarios ni likes: la conversación sobre tu actividad ocurre en las instancias de los demás y no la ves en la tuya.
- Somos responsables de los bugs y de los arreglos de seguridad del código copiado: ya no llegan solos desde upstream.
- Upstream lo mantiene prácticamente una persona y sin versionado; su código venía con tests rotos (corregidos en la copia).
- El dominio queda fijo para siempre. Migrar requeriría implementar `Move` (fuera del v1).

## Alternativas consideradas

| Alternativa | Razón de descarte |
|-------------|-------------------|
| Instancia multiusuario | Agrega registro, roles y moderación desde el día 1; la visión es "una instancia por persona" |
| v1 de un usuario con `account_id` en todas las tablas | Complejidad "para después" sin un caso de uso concreto |
| Solo perfil, sin actividad | Presencia en el fediverso sin nada que seguir |
| Proyectos como objetos ricos en v1 | Depende del modelo Proyecto/Ejecución, aún sin decidir; Mastodon los vería degradados |
| Procesar replies y likes | Obliga a moderar (bloquear, borrar) desde el v1 |
| Implementación AP mínima propia | Se prioriza robustez sobre aprendizaje; reimplementar el vocabulario y sus casos raros es caro |
| `github.com/go-fed/activity` upstream | Sin mantenimiento desde 2022 |
| Fork de go-fed que mantiene GoToSocial (`code.superseriousbusiness.org/activity`) | API generada enorme y abstracta; se prefirió el modelo más simple de go-ap |
| go-ap como dependencia (`go get`) | Sin versiones estables y con un solo mantenedor: cada actualización es un commit arbitrario |
| Reutilizar código de GoToSocial | Obliga a AGPL y arrastra complejidad multiusuario |

## Pendiente de resolver en implementación

- ~~Verificar con un spike que las HTTP Signatures de `httpsig` y los tipos de go-ap interoperan con Mastodon y GoToSocial.~~ Hecho en gremio#1 y gremio#3.
- ~~Política de reintentos de la cola de entrega~~ Resuelto en gremio#7 (ver la sección 5). "Dar por muerta" una instancia es por entrega (12 intentos), no hay un estado por servidor; agregarlo si muchas entregas a un servidor caído empiezan a pesar.

## Links relacionados

- KB: `/Users/lx-duv0-x/Documents/ia-kb/drafts/plataforma-perfiles/README.md`
- Referencia local de uso: `/Users/lx-duv0-x/Documents/Repositories/echo.duv0-x.cloud` (`go.mod`)
- Próximos ADRs: modelo Proyecto/Ejecución, modelo de perfil v1, licencia.
