---
title: "Dojo: puntaje de Entrenamiento ganado con retos, con decaimiento por volatilidad"
number: 13
date: 2026-11-07
domain: architecture
status: "propuesto"
tags: [dojo, scoring, retos, decay, credibility, ficha]
---

# ADR-013: Dojo y puntaje de Entrenamiento

## Contexto

El objetivo de Gremio es que quien recluta crea lo que ve. Hoy cada sub-atributo de la ficha ([ADR-010](ADR-010-ficha-de-jugador.md)) tiene un puntaje de 0 a 99 que **declara el dueño** y respalda con logros hechos en empresas donde trabajó. Es honesto en lo que dice, pero un tercero difícilmente puede corroborarlo, y nada muestra si ese conocimiento sigue vigente.

El dueño quiere una segunda señal que **no se declara: se gana**. La gana resolviendo **retos** en un "dojo": proyectos o ejercicios con enunciado, nivel y criterios de terminado ([ADR-002](ADR-002-proyecto-y-ejecucion.md)), que puede proponer un modelo de IA a través de los plugins ([ADR-012](ADR-012-plugins.md)). Esa señal tiene que reflejar dos cosas:
- **la dificultad** de lo demostrado;
- **la vigencia**: lo que no se practica se va perdiendo, más rápido en áreas que cambian constantemente (programación agéntica, servicios de AWS, APIs nuevas de Kubernetes) que en conocimientos estables (administración Linux).

El dueño insiste en que este motor es la base de la credibilidad del perfil: tiene que ser **claro, público y recalculable** por cualquiera.

## Decisión

### 1. Dos barras por sub-atributo: Declarado y Entrenamiento

- **Declarado** es el puntaje actual de ADR-010, sin cambios: lo pone el dueño y lo respalda con logros. Sigue siendo la base del puntaje del eje, del OVR por rol y del radar.
- **Entrenamiento** es un segundo puntaje de 0 a 99 que **solo se gana en el dojo**. El dueño no lo puede editar. Se muestra como una segunda barra de otro color, con la leyenda una vez por tarjeta ("● Declarado ● Entrenamiento"), sus números y un tooltip con la última práctica y la volatilidad. No depende solo del color: lleva texto y `sr-only`.
- **Arranca vacío.** Un sub-atributo sin retos resueltos muestra "Sin entrenamiento todavía", no un 0.
- **No se mezclan:** el OVR y el radar siguen calculándose solo con lo declarado. Un "OVR de entrenamiento" se decidirá aparte.

### 2. Catálogo común de habilidades con volatilidad

- Gremio trae un **catálogo de habilidades** (`internal/profile/skills_catalog.go`), como el de posiciones de [ADR-011](ADR-011-catalogo-de-posiciones.md). Cada entrada tiene un id estable (`linux-admin`, `kubernetes`, `aws-compute`, `agentic-coding`…), un nombre y una **volatilidad**. La volatilidad la fija el catálogo, no el dueño:

  | Volatilidad | Vida media (H) | Ejemplos |
  |---|---|---|
  | Estable | 5 años (1826 días) | Administración Linux, redes, SQL, Git |
  | Media | 2 años (730 días) | Kubernetes, Terraform, servicios de AWS, CI/CD |
  | Volátil | 9 meses (274 días) | Programación agéntica, herramientas de LLM, MCP |

- Cada sub-atributo del perfil puede enlazarse a una entrada del catálogo (`subs[].skill`). **Solo los sub-atributos enlazados tienen Entrenamiento.** Los que no están enlazados muestran únicamente lo declarado.
- Los retos declaran qué habilidades del catálogo ejercitan, así que lo que suma un reto es independiente de cómo el dueño nombró sus sub-atributos.
- El catálogo crece por PR. Los ids no se renombran ni se borran. Cambiar la volatilidad de una habilidad es un cambio de reglas y se versiona (§6).

### 3. Lo que suma un reto

Un reto suma cuando su **ejecución** (ADR-002) pasa a `done` con una **verificación** válida (§4). Por cada habilidad que ejercita:

```
ganancia = base(nivel) × verificación × (1 − actual / techo(nivel))
```

- `actual` es el valor de Entrenamiento de esa habilidad en el momento de terminar, ya con el decaimiento aplicado (§5).
- **Base y techo por nivel** (los niveles N1–N4 son los de ADR-002):

  | Nivel | Base | Techo |
  |---|---|---|
  | N1 Fundamentos | 8 | 40 |
  | N2 | 12 | 60 |
  | N3 | 16 | 80 |
  | N4 Experto | 20 | 99 |

- **Rendimientos decrecientes:** cuanto más cerca del techo, menos suma. Si `actual ≥ techo(nivel)`, el reto suma 0 en esa habilidad: para pasar de 40 hacen falta retos N2 o más, y para pasar de 80, N4.
- **Un reto suma una sola vez por persona.** Repetir el mismo reto, en cualquier versión, no suma.
- El resultado se redondea al entero más cercano y nunca pasa de 99.

### 4. Verificación

Cada ejecución terminada declara cómo se verificó. Eso fija el multiplicador:

| Verificación | Multiplicador | Qué es |
|---|---|---|
| Automática | 1.0 | El reto trae comprobaciones ejecutables (tests, un endpoint que responde un contrato, un despliegue que pasa un chequeo), y Gremio o un verificador las corre y guarda el resultado |
| Evidencia | 0.6 | Hay entregables públicos (repo, PR, informe, demo) que cualquiera puede revisar, sin comprobación automática |
| Atestada | 1.0 | Una persona de otra instancia revisó la evidencia y firmó una atestación (§7). Solo completa una ejecución con evidencia: no sustituye a la evidencia |

- **Sin verificación no hay ganancia.** Marcar "lo hice" no suma.
- La verificación queda a la vista junto a cada punto.

### 5. Decaimiento y piso

Sin entrenar, el Entrenamiento baja hacia un **piso**, a la velocidad que fija la volatilidad de la habilidad:

```
piso  = 0.4 × máximo histórico alcanzado en esa habilidad
valor(t) = piso + (v₀ − piso) × 0.5 ^ (Δt / H)
```

- `v₀` es el valor justo después del último reto que sumó en esa habilidad, `Δt` son los días desde ese reto y `H` es la vida media de §2.
- **El piso reconoce que lo aprendido no desaparece del todo:** quien llegó a 80 nunca baja de 32.
- **Ejemplo:**
  - Programación agéntica (volátil, H = 274 días) en 80, con piso 32: queda en 56 a los 9 meses y en 44 a los 18 meses.
  - El mismo 80 en administración Linux (estable, H = 1826 días) queda en 74 al año.
  - Un reto N3 con verificación automática, hecho cuando la programación agéntica estaba en 44, la deja en 44 + 16 × (1 − 44/80) ≈ 51.

### 6. Un libro de eventos, recalculable por cualquiera

- El Entrenamiento **no se guarda como número**. Se calcula a partir de un **libro de eventos** inmutable: cada ejecución terminada, con su reto (id y versión), su nivel, las habilidades que ejercita, la verificación y la fecha. Así cualquiera puede recalcular el valor de hoy.
- **Las reglas tienen versión** (`rules: "dojo-1"`), con las tablas de §2 a §5. Si las reglas cambian, se recalcula todo el libro con la versión nueva, y la ficha dice con qué versión se calculó.
- Cada punto enlaza al reto (enunciado público e inmutable, ADR-002 §3), a la ejecución con su evidencia y a la verificación.
- **El libro se publica:** los eventos terminados se federan como actividad del dueño (ver gremio#119 y el timeline), así que también otras instancias pueden recalcular.

### 7. Atestaciones

- Una atestación es un objeto firmado por el actor de **otra instancia** que dice "revisé la ejecución X del reto Y y cumple su Definition of Done". Llega por ActivityPub y queda en el libro.
- El modelo exacto (tipo de actividad, firma del objeto, revocación) queda para un ADR propio, junto con el módulo de equipos (ADR-010 §2).

### 8. Quién propone los retos y quién fija su nivel

- Los retos los proponen personas o un modelo de IA mediante los plugins. Si los genera una IA, nacen como **borrador** y una persona los revisa y publica (ADR-002 §4). **La IA no puntúa a nadie:** propone ejercicios.
- **Riesgo abierto: el nivel lo declara el autor del reto.** Si el dueño escribe sus propios retos N4 fáciles y los verifica con evidencia, se infla. Mitigaciones para la v1:
  - los retos propios del dueño muestran que son propios;
  - un reto de otra persona (otra instancia), o uno con verificación automática, pesa más que uno propio sin automatizar.

  La calibración de niveles (retos compartidos que resuelven varias personas y estadística de cuántos los logran) queda para una versión posterior.

## Consecuencias positivas

- El reclutador ve dos señales separadas: lo que la persona dice que hizo (respaldado con logros) y lo que demostró hace poco en el dojo, con su fecha y su verificación. La diferencia entre las dos barras dice algo por sí misma.
- El decaimiento por volatilidad refleja cómo envejece de verdad el conocimiento técnico.
- Las reglas son públicas, versionadas e iguales para todas las instancias, y el valor se recalcula desde un libro inmutable: nadie ajusta su número a mano.
- La IA solo propone; los puntos salen de verificaciones. Eso baja el riesgo regulatorio de "calificar personas con IA" (ver `ideas-aparcadas` en la KB).

## Consecuencias negativas

- Hay que mantener el catálogo de habilidades y sus volatilidades, y los sub-atributos no enlazados no tienen Entrenamiento.
- Los números de las tablas son una primera calibración, sin datos reales. Se ajustarán con una versión nueva de las reglas.
- La verificación automática exige que los retos traigan comprobaciones ejecutables, y no todos los retos pueden tenerlas.
- Los retos que se escribe el propio dueño son la vía más fácil para inflar el número. La v1 lo mitiga mostrando el origen, pero no lo resuelve.

## Alternativas consideradas

| Alternativa | Por qué no |
|-------------|-----------|
| Reemplazar lo declarado por lo ganado | Deja vacío cualquier perfil nuevo y borra una señal válida: lo hecho en empresas |
| Que la barra de Entrenamiento arranque igual a lo declarado y decaiga (opción B) | Mezcla las dos fuentes: un perfil nuevo vería bajar barras que nunca entrenó |
| Que una IA califique la solución | Es lo que el AI Act de la UE trata como alto riesgo en reclutamiento, y quita transparencia |
| Una sola volatilidad para todo | Linux y la programación agéntica no envejecen igual |
| Guardar el número y actualizarlo | No se puede auditar ni recalcular cuando cambian las reglas |

## Links relacionados

- [ADR-002](ADR-002-proyecto-y-ejecucion.md): Proyecto (reto) y Ejecución.
- [ADR-010](ADR-010-ficha-de-jugador.md): ficha, sub-atributos y lo declarado.
- [ADR-011](ADR-011-catalogo-de-posiciones.md): catálogo de posiciones.
- [ADR-012](ADR-012-plugins.md): plugins de IA que proponen retos.
- gremio#119.
