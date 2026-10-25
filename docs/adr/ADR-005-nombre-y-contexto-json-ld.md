---
title: "Nombre del proyecto y URL permanente del contexto JSON-LD"
number: 5
date: 2026-10-24
domain: governance
status: "aceptado"
tags: [naming, json-ld, w3id, activitypub, permanent-identifier]
---

# ADR-005: Nombre del proyecto y URL permanente del contexto JSON-LD

## Contexto

El repo nació como `reverse-mini-ats`, por el diferencial original (el candidato dueño del proceso de selección). El alcance actual es perfil + proyectos, y ese nombre ya no lo describe.

Además, [ADR-002](ADR-002-proyecto-y-ejecucion.md) define propiedades propias (`level`, `definitionOfDone`…) bajo un contexto JSON-LD. La URL de ese contexto viaja dentro de cada proyecto publicado y otras instancias la guardan, así que **no puede cambiar nunca**: si deja de responder, los proyectos que ya circulan quedan con su vocabulario roto.

## Decisión

### 1. Nombre: Gremio

- Los gremios eran la red de profesionales de un oficio, que certificaba el progreso aprendiz → oficial → maestro con una *obra maestra*. Calza con los proyectos por niveles (N1 a N4) y con armar equipos en la fase siguiente.
- Repo: `duv0-x/gremio` (renombrado desde `reverse-mini-ats` el 2026-10-24; GitHub redirige el nombre viejo). Módulo Go: `github.com/duv0-x/gremio`.
- Al revisarlo, no había proyectos del fediverso ni de perfiles profesionales con ese nombre.
- **Actualización 2026-10-24: "Gremio" es el nombre clave del proyecto.** El nombre público puede cambiar más adelante. La URL del contexto (`https://w3id.org/gremio/ns/v1`) **no cambia aunque cambie el nombre**: es un identificador permanente, no una marca.

### 2. Contexto JSON-LD en w3id.org

- URL permanente: **`https://w3id.org/gremio/ns/v1`**.
- [w3id.org](https://w3id.org) es un servicio de identificadores permanentes del W3C Permanent Identifier Community Group. Una ruta se registra con un PR a [`perma-id/w3id.org`](https://github.com/perma-id/w3id.org) y redirige adonde viva el contenido hoy. Si cambia el hosting, se actualiza la redirección con otro PR y la URL publicada no cambia.
- Es el mismo mecanismo que usa el fediverso para `https://w3id.org/security/v1`, presente en el `@context` de cada Actor de Mastodon y GoToSocial.
- **`v1` en la ruta**: un cambio incompatible del vocabulario se publica como `v2` sin romper los objetos que usan `v1`. Agregar propiedades nuevas y opcionales no requiere nueva versión.
- El documento del contexto es CC0 ([ADR-004](ADR-004-licencia.md)) y vive en `docs/spec/`.

## Consecuencias positivas

- El nombre cuenta la propuesta: oficio, niveles, obra maestra, comunidad.
- La URL del contexto no depende de ningún dominio ni hosting que haya que mantener para siempre.
- Versionar el contexto desde el inicio permite evolucionar el vocabulario sin romper la red.

## Consecuencias negativas

- "Gremio" es una palabra común en español: buscar el proyecto en internet será más difícil que con un nombre inventado.
- Registrar la ruta en w3id.org es un PR público: el nombre queda visible aunque el repo siga privado.
- Dependemos de que la comunidad de w3id.org siga operando (lleva años en uso, y vocabularios críticos del fediverso dependen de ella).

## Alternativas consideradas

| Alternativa | Razón de descarte |
|-------------|-------------------|
| Mantener `reverse-mini-ats` | Describe un módulo aparcado, no el alcance actual |
| Oficio | Muy usado como palabra suelta; menos evocador de red y niveles |
| Guildhall | En inglés y con choques menores (gestor de paquetes de Guile, motor de juegos) |
| Contexto en un dominio propio | Hay que renovarlo para siempre; si se pierde, se rompe la federación |
| Contexto en un subdominio de `duv0-x.cloud` | Ata el vocabulario de toda la red al dominio personal del autor |

## Pendiente de resolver (antes de federar el primer proyecto)

- **Dónde se sirve el contexto hoy.** El repo es privado, y GitHub Pages en un repo privado no sirve en el plan gratuito. Opciones: un repo público solo para la spec, publicar `gremio` como público, o servirlo desde una infraestructura existente.
- Escribir `docs/spec/` (vocabulario y `context.jsonld`) y abrir el PR de registro en `perma-id/w3id.org`.

## Links relacionados

- [ADR-002](ADR-002-proyecto-y-ejecucion.md): las propiedades que define el contexto.
- [ADR-004](ADR-004-licencia.md): CC0 para la especificación.
