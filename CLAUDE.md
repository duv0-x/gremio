# CLAUDE.md — Gremio

El dueño es Duván Ballén (Senior Platform Engineer, aprendiendo Go). Comunícate en español, con términos técnicos en inglés.

## Estado: fase de decisiones

El único código es `internal/goap/`: copia propia del núcleo de go-ap (MIT) que mantenemos nosotros; ver su `README.md` y no borrar sus `LICENSE`. Antes de construir, cada decisión de diseño, arquitectura o stack se documenta como ADR en `docs/adr/ADR-NNN-titulo.md`. No escribas código ni crees issues hasta que Duván lo pida.

## Contexto completo

Toda la conversación de diseño vive en la KB, no aquí:

- `/Users/lx-duv0-x/Documents/ia-kb/drafts/plataforma-perfiles/README.md`: visión, alcance acordado y preguntas abiertas.
- `/Users/lx-duv0-x/Documents/ia-kb/drafts/plataforma-perfiles/ideas-aparcadas.md`: ideas fuera de alcance (mini-ATS, capacidades vs. títulos, reputación, armador de equipos, agente evaluador), riesgos y observaciones de los mockups.
- `/Users/lx-duv0-x/Documents/ia-kb/drafts/plataforma-perfiles/handoff-original.md`: traspaso de la primera sesión de diseño.

## Alcance acordado (2026-10-24)

- **Una instancia por usuario, compatible con ActivityPub** (estilo GoToSocial: un binario, fácil de desplegar).
- **Módulo Perfil**: público + panel de configuración.
- **Módulo Proyectos**: crear, escoger y proponer proyectos; posible generación con IA; la evaluación se estudia después.
- Fase siguiente: armar equipos.

## Arquitectura

Go puro (`CGO_ENABLED=0`), un binario con todo embebido; binarios + imagen OCI con GoReleaser (`docs/adr/ADR-007-lenguaje-y-distribucion.md`, aceptado). Web renderizada en el servidor con templ + htmx y estilos con Tailwind standalone (`docs/adr/ADR-008-frontend.md`, aceptado). Solo SQLite (ncruces + sqlc + goose, `docs/adr/ADR-006-almacenamiento.md`, aceptado). El código generado (sqlc, templ, CSS) se commitea. La prioridad es una plataforma descentralizada y robusta (por eso Go), con código idiomático. Referencia local de Go + ActivityPub: el fork de GoToSocial en `/Users/lx-duv0-x/Documents/Repositories/echo.duv0-x.cloud` (AGPL, compatible con la licencia del proyecto).

## Go

Módulo `github.com/duv0-x/gremio`, Go 1.26. En esta Mac el `go` de `/usr/local/go` es 1.23: usar el de brew con `export PATH=/opt/homebrew/bin:$PATH`. Verificar con `go vet ./... && go test ./...`.

## Decisiones pendientes (candidatas a ADR)

1. ~~Modelo de instancia y qué se federa por ActivityPub~~ → `docs/adr/ADR-001-instancia-y-federacion.md` (aceptado).
2. ~~Modelo Proyecto vs. Ejecución~~ → `docs/adr/ADR-002-proyecto-y-ejecucion.md` (aceptado).
3. ~~Modelo de perfil v1~~ → `docs/adr/ADR-003-perfil-v1.md` (aceptado).
4. ~~Licencia~~ → `docs/adr/ADR-004-licencia.md` (aceptado): AGPL-3.0-or-later, `internal/goap/` MIT, spec CC0, contribuciones con DCO (`git commit -s`).

## Nombre y vocabulario

El proyecto se llama **Gremio** (antes `reverse-mini-ats`). El contexto JSON-LD propio es `https://w3id.org/gremio/ns/v1` y no puede cambiar nunca. Ver `docs/adr/ADR-005-nombre-y-contexto-json-ld.md` (aceptado).

## Mockups

`docs/mockups/*.dc.html`: markup en `<x-dc>` con `{{holes}}`, `<sc-for>`, `<sc-if>` y una clase `Component extends DCLogic`. Úsalos como especificación de layout y contenido. `Main` (perfil) y `Retos` (base del módulo Proyectos) aplican al alcance actual; `Pipeline` y `Equipos` reflejan ideas aparcadas.
