---
title: "Medallas: insignias ganadas y verificables, en lugar de rasgos autodeclarados"
number: 15
date: 2026-11-07
domain: architecture
status: "aceptado"
tags: [medallas, badges, dojo, github, credibility, ficha]
---

# ADR-015: Medallas

## Contexto

La ficha muestra **rasgos** ([ADR-010](ADR-010-ficha-de-jugador.md)): insignias hexagonales con ícono y color ("Automatizadora") que el dueño **declara**. El dueño quiere que las insignias sean **medallas ganadas de verdad**, y además divertidas. Por ejemplo:
- una medalla si es colaborador comprobado del repo de Gremio en GitHub;
- otra si termina tres retos fáciles en un mismo día.

Como con el Entrenamiento ([ADR-013](ADR-013-dojo-y-entrenamiento.md)), una medalla solo da credibilidad si cualquiera puede comprobar por qué se ganó.

## Decisión

### 1. Un motor aparte que lee el libro de eventos

- Las medallas **no son puntaje**: son hechos puntuales ("cumplió tal condición en tal fecha") y **no se pierden**, aunque el Entrenamiento decaiga. Por eso van en un motor propio (`internal/medals`), separado del motor de puntaje, que lee el **mismo libro de eventos** del dojo y fuentes externas verificadas.
- Una medalla ganada se registra como **evento en el libro** (`badgeAwarded`, con el id de la medalla, la regla, la fecha y la prueba). Sale en Actividad ([ADR-014](ADR-014-actividad.md)) y se federa.
- Las reglas se evalúan después de cada evento del libro, y las externas también periódicamente, en `serve`. Son deterministas: el mismo libro produce las mismas medallas.

### 2. Catálogo de medallas en el código

- Cada medalla tiene un id estable, un nombre, una descripción de cómo se gana, un ícono de Lucide y un color de la paleta de rasgos (`TraitColors`, DESIGN.md), y su regla. El catálogo crece por PR, y los ids no se renombran ni se borran.
- Hay dos clases:
  - **del dojo:** su regla se calcula solo con el libro de eventos;
  - **externas:** su regla consulta un servicio externo con una identidad demostrada (§3).
- **Catálogo inicial** (v1):

  | Id | Nombre | Cómo se gana |
  |----|--------|--------------|
  | `cinturon-blanco` | Cinturón blanco | Terminar el primer reto verificado |
  | `triple-kata` | Triple kata | Terminar 3 retos N1 verificados el mismo día (hora de la zona del perfil) |
  | `primer-n4` | Contra el maestro | Terminar el primer reto N4 verificado |
  | `racha-7` | Siete amaneceres | Terminar al menos un reto verificado en 7 días distintos seguidos |
  | `polimata` | Polímata | Sumar Entrenamiento en habilidades de 5 atributos distintos |
  | `forjador` | Forjador del Gremio | Ser contribuidor comprobado de `duv0-x/gremio` en GitHub (externa) |

  Los nombres siguen la metáfora del dojo y el gremio. Se pueden ajustar antes de publicar la v1.

### 3. Identidad externa demostrada: GitHub

- **Que un correo coincida con los commits no prueba nada**, porque cualquiera puede firmar commits con cualquier correo. La identidad se demuestra en las dos direcciones:
  1. El dueño habilita el plugin **GitHub** ([ADR-012](ADR-012-plugins.md)) con su usuario. El token es opcional y solo sirve para la cuota de la API.
  2. El chequeo del plugin pide `GET https://api.github.com/users/<usuario>` y exige que el campo `blog` (el sitio web del perfil de GitHub) apunte a la instancia (`https://<host>` o `https://<host>/@usuario`). Solo el dueño de la cuenta de GitHub puede poner eso.
- Con la identidad demostrada, la regla de `forjador` consulta `GET /repos/duv0-x/gremio/contributors` y busca ese usuario. La prueba guardada es el usuario, la fecha y la URL consultada.
- Si la identidad deja de comprobarse (cambió el `blog` o se deshabilitó el plugin), las medallas externas ya ganadas se marcan "sin comprobar". No se borran, porque la medalla registra un hecho pasado.

### 4. Las medallas reemplazan a los rasgos en la ficha

- La fila de **Rasgos** de la ficha pasa a ser **Medallas**: con el mismo emblema hexagonal, pero solo con medallas ganadas. El tooltip dice cómo se ganó, la fecha y, si es externa, cómo se comprobó.
- Los rasgos autodeclarados **dejan de mostrarse** en la ficha. Los datos se conservan en el perfil (`gremio.traits`) para no perder nada al exportar, y su editor sale del panel.
- Sin medallas, la fila no aparece.

## Consecuencias positivas

- Las insignias pasan de "lo que digo de mí" a "lo que se puede comprobar", que es el objetivo de Gremio.
- Es divertido y motiva a entrenar en el dojo sin tocar el puntaje.
- Las medallas externas abren la puerta a otras fuentes verificadas: certificados con URL de validación, Credly.

## Consecuencias negativas

- Al principio la ficha tiene menos insignias, solo las ganadas.
- Las externas dependen de la disponibilidad y la cuota de GitHub.
- Hay que mantener el catálogo y sus reglas.

## Alternativas consideradas

| Alternativa | Por qué no |
|-------------|-----------|
| Medallas dentro del motor de puntaje | Mezcla hechos permanentes con un número que decae |
| Verificar "Forjador" por el correo de los commits | El correo de un commit se falsifica trivialmente |
| Mantener los rasgos autodeclarados junto a las medallas | El dueño quiere insignias ganadas, no declaradas |

## Links relacionados

- [ADR-010](ADR-010-ficha-de-jugador.md), [ADR-012](ADR-012-plugins.md), [ADR-013](ADR-013-dojo-y-entrenamiento.md) y [ADR-014](ADR-014-actividad.md).
