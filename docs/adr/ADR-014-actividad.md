---
title: "Actividad: una línea de tiempo de hechos verificables, federada, sin posts libres"
number: 14
date: 2026-11-07
domain: architecture
status: "aceptado"
tags: [actividad, timeline, activitypub, dojo, medallas, credibility]
---

# ADR-014: Actividad

## Contexto

Gremio llegó a tener una pestaña **Notas** con posts libres que el dueño publicaba en el fediverso (gremio#112, gremio#115). El dueño la quitó (gremio#121): no quiere que Gremio se llene de "posts basura" como los de LinkedIn, sino de **actividad real**: retos empezados y terminados en el dojo ([ADR-013](ADR-013-dojo-y-entrenamiento.md)) y medallas ganadas ([ADR-015](ADR-015-medallas.md)). Esa actividad tiene que verse en el perfil, llegar a los seguidores por ActivityPub y quedar firmada por la instancia.

## Decisión

### 1. Una pestaña, Actividad, alimentada solo por hechos

- La pestaña **Actividad** (`/@usuario/actividad`) es una línea de tiempo, de lo más nuevo a lo más viejo, de **eventos públicos**:
  - **empezó un reto**: una ejecución que pasa a `in_progress` y es pública (ADR-002 §5);
  - **terminó un reto**: muestra el nivel, la verificación, las habilidades y lo que sumó en Entrenamiento (ADR-013);
  - **ganó una medalla** (ADR-015);
  - más adelante, **recibió una atestación** (ADR-013 §7).
- **No hay posts libres:** ningún texto lo escribe el dueño a mano. Cada evento sale del libro de eventos del dojo o de las medallas, y su texto lo arma Gremio a partir de los datos ("Terminó el reto *Desplegar un operador de Kubernetes* (N3, verificación automática): +7 en Kubernetes").
- Cada evento enlaza a lo que lo respalda: la página pública del reto, la ejecución con su evidencia, la medalla.
- **La pestaña aparece cuando hay al menos un evento público.** Sin eventos no se muestra, y `/@usuario/actividad` da 404.
- Lo privado no sale: las ejecuciones privadas (ADR-002 §5) no generan eventos visibles ni federados. Si una ejecución se publica después, sus eventos aparecen con su fecha original.

### 2. Federación: cada evento es un `Note` generado

- Cada evento público se publica como `Create` de un `Note` a los seguidores, con el mecanismo que ya existe (`PublishNote`, la tabla `notes`, el outbox y la cola de entrega de [ADR-001](ADR-001-instancia-y-federacion.md)). Mastodon y demás lo muestran como texto con enlace.
- El `Note` lleva, además, propiedades propias bajo el contexto JSON-LD de Gremio (`https://w3id.org/gremio/ns/v1`, [ADR-005](ADR-005-nombre-y-contexto-json-ld.md)):
  - `gremio:event`: el tipo (`challengeStarted`, `challengeCompleted`, `badgeAwarded`);
  - `gremio:ref`: el id del objeto respaldado (reto, ejecución, medalla);
  - `gremio:rules`: la versión de las reglas del dojo.

  Otra instancia de Gremio puede así reconstruir el libro y recalcular (ADR-013 §6).
- El `Note` no se edita. Si una medalla o un punto se revocan, se publica un `Delete` del `Note` y un evento de revocación.

### 3. Firma

- Toda entrega va firmada con HTTP Signatures y la llave de la instancia, como hoy. El `id` de cada `Note` vive en la instancia de origen, así que cualquiera puede comprobar que lo publicó ese actor.
- **Integridad del objeto** (una firma dentro del propio objeto, verificable aunque se copie a otro lado, como FEP-8b32): queda para un ADR propio. Necesita una llave Ed25519 aparte de la RSA actual.

### 4. Sin configuración

- No hay casilla para mostrar u ocultar la pestaña: si hay actividad pública, se muestra. Lo que el dueño controla es qué ejecuciones publica (ADR-002).

## Consecuencias positivas

- El perfil y los seguidores ven solo hechos con respaldo, nunca opiniones ni anuncios.
- Reutiliza la federación existente: no hay un protocolo nuevo.
- Las propiedades de Gremio en el `Note` hacen público el libro del dojo para otras instancias.

## Consecuencias negativas

- Mientras el dojo no tenga retos, la pestaña no aparece.
- Los textos generados son menos expresivos que un post escrito a mano. Es la idea.
- La firma del objeto en sí queda pendiente.

## Alternativas consideradas

| Alternativa | Por qué no |
|-------------|-----------|
| Notas libres (gremio#112 y #115) | El dueño las quitó: posts sin respaldo |
| Federar los eventos como actividades propias (`gremio:ChallengeCompleted`) | Mastodon no las muestra; un `Note` con extensión se ve en todas partes y se entiende en Gremio |
| Un timeline también con lo que publican las cuentas seguidas | Sería una red social; Gremio es un perfil |

## Links relacionados

- [ADR-001](ADR-001-instancia-y-federacion.md), [ADR-002](ADR-002-proyecto-y-ejecucion.md), [ADR-005](ADR-005-nombre-y-contexto-json-ld.md), [ADR-013](ADR-013-dojo-y-entrenamiento.md) y [ADR-015](ADR-015-medallas.md).
- gremio#121: se quitaron las notas libres.
