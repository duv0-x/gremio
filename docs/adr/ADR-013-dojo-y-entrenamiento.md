---
title: "Dojo: puntaje de Entrenamiento ganado con retos, con topes de confianza y decaimiento por volatilidad"
number: 13
date: 2026-11-07
domain: architecture
status: "aceptado"
tags: [dojo, scoring, retos, decay, credibility, ficha]
---

# ADR-013: Dojo y puntaje de Entrenamiento

## Contexto

El objetivo de Gremio es que quien recluta crea lo que ve. Hoy cada sub-atributo de la ficha ([ADR-010](ADR-010-ficha-de-jugador.md)) tiene un puntaje de 0 a 99 que **declara el dueño** y respalda con logros hechos en empresas donde trabajó. Un tercero difícilmente puede corroborarlo, y nada muestra si ese conocimiento sigue vigente.

El dueño quiere una segunda señal que **no se declara: se gana** resolviendo **retos** en un "dojo" (Proyecto y Ejecución de [ADR-002](ADR-002-proyecto-y-ejecucion.md)), que puede proponer un modelo de IA mediante los plugins ([ADR-012](ADR-012-plugins.md)). Esa señal tiene que reflejar dos cosas:
- **la dificultad** de lo demostrado;
- **la vigencia**: lo que no se practica se pierde, más rápido en áreas que cambian constantemente (programación agéntica, servicios nuevos de AWS, APIs nuevas de Kubernetes) que en conocimientos estables (administración Linux).

Tiene que ser **claro, público y recalculable** por cualquiera.

Una revisión independiente del primer borrador (2026-11-07) simuló las reglas y encontró tres problemas:
- **Las verificaciones débiles llegaban igual de alto:** 20 retos propios con "evidencia" llevaban una habilidad a 91 en 20 días.
- **El número medía volumen, no dificultad:** un N4 verificado daba 20/99.
- **El resultado dependía del orden** de los retos y del redondeo.

Esta versión corrige todo eso.

## Decisión

### 1. Dos barras por sub-atributo: Declarado y Entrenamiento

- **Declarado** es el puntaje de ADR-010, sin cambios: lo pone el dueño y lo respalda con logros. Sigue siendo la base del puntaje del eje, del OVR por rol y del radar.
- **Entrenamiento** es un segundo puntaje de 0 a 99 que **solo se gana en el dojo**, y que el dueño no puede editar. Arranca vacío: un sub-atributo sin retos muestra "Sin entrenamiento todavía", no un 0.
- **No se mezclan.** El OVR y el radar usan solo lo declarado.
- **Sin rankings:** Gremio no ofrece ordenar ni filtrar personas por Entrenamiento, ni en el directorio ni en el armador de equipos (RGPD art. 22; ver §9).

### 2. Catálogo común de habilidades con volatilidad

- Gremio trae un **catálogo de habilidades** (`internal/dojo/skills.go`). Cada entrada tiene un id estable (`linux-admin`, `kubernetes`, `aws-core`, `aws-new-services`, `agentic-coding`…), un nombre, alias para reconocer nombres libres y una **volatilidad**. La volatilidad la fija el catálogo, no el dueño:

  | Volatilidad | Vida media H | Ejemplos |
  |---|---|---|
  | Estable | 1826 días (5 años) | Administración Linux, redes, SQL, Git |
  | Media | 730 días (2 años) | Kubernetes core, Terraform, AWS core (EC2, S3, IAM), CI/CD |
  | Volátil | 274 días (9 meses) | Programación agéntica, herramientas de LLM, MCP, servicios nuevos de AWS, APIs beta de Kubernetes |

- Cada volatilidad se justifica en el catálogo con un indicador público del ritmo de cambio de la tecnología (por ejemplo, Kubernetes publica 3 versiones al año).
- Cada sub-atributo del perfil se puede enlazar a **una** habilidad del catálogo con `subs[].dojoSkill`. Un sub-atributo enlazado tiene Entrenamiento; uno sin enlazar muestra solo lo declarado. La misma habilidad no se repite en dos sub-atributos.
  - El nombre `dojoSkill` es deliberado: `attributes[].skills` ya existe y son las skills de JSON Resume, y el contexto JSON-LD no cambia nunca ([ADR-005](ADR-005-nombre-y-contexto-json-ld.md)).
- La barra muestra el nombre de la habilidad del catálogo si no coincide con el del sub-atributo.
- El catálogo crece por PR. Los ids no se renombran ni se borran. Una habilidad que no está en el catálogo se ignora, y se recupera hacia atrás cuando una versión de las reglas la agrega.

### 3. Bandas, entrada de banda y nivel por habilidad

- **Las bandas son las mismas para todos:**

  | Nivel | Entrada | Techo | Qué significa |
  |---|---|---|---|
  | N1 Fundamentos | 0 | 40 | Lo hizo con guía |
  | N2 | 40 | 60 | Lo resuelve solo |
  | N3 | 60 | 80 | Nivel senior: decide y opera en producción |
  | N4 Experto | 80 | 99 | Referente |

  El 99 es asintótico: se acerca y no se toca.
- **Un reto declara, por cada habilidad que ejercita, el nivel en esa habilidad.** Ese nivel no puede pasar el nivel del reto, y un reto ejercita **3 habilidades como máximo**.
- **Regla de ganancia** (una por habilidad):

  ```
  L     = min(nivel de la habilidad en el reto, tope de confianza de la ejecución)   (§4)
  si actual < techo(L):
      b     = max(actual, entrada(L))
      nuevo = b + 0.2 × (techo(L) − b)
  si no: no suma
  ```

  En una línea: **"cada reto te lleva al menos a la entrada de su banda y cierra el 20 % de lo que falta hasta el techo"**. Un N3 verificado deja la barra en 64; cuatro N3 en un año la mantienen en la banda N3; para pasar de 80 hacen falta retos N4.
- **Un reto suma una sola vez por persona**, contando todas sus versiones y su linaje: `basedOn` de ADR-002, de modo que las copias o forks del mismo reto cuentan como uno. Retirar el reto y volver a hacerlo no suma de nuevo (§6).

### 4. Topes de confianza en lugar de multiplicadores

La forma de verificar **no multiplica** la ganancia: **tapa el nivel que puede alcanzarse**.

| Confianza | Tope | Qué es |
|---|---|---|
| Práctica propia | N2 (60) | Un reto que no es del catálogo revisado, **incluidos los generados por IA**, o un reto sin checks reproducibles. Que la propia instancia corra los checks es autoverificación |
| Reproducible | N3 (80) | Un reto del **catálogo revisado** (§8), con la entrega fijada a un commit y checks públicos que cualquiera puede volver a correr con `gremio dojo verify` y obtener el mismo resultado |
| Verificada por terceros | N4 (99) | Un reto del catálogo revisado cuyos checks corrió **y firmó alguien distinto del dueño**: una attestation de CI con identidad del workflow (Sigstore) o una instancia verificadora con su llave |

- **Sin verificación no hay ganancia.** Marcar "lo hice" no suma.
- **Mientras no exista el catálogo revisado con checks reproducibles, nadie pasa de 60.** Es así a propósito, y la ficha lo dice.
- **Las atestaciones de personas** ("trabajé con él") **no cuentan para el puntaje** hasta que tengan su propio ADR. Ese ADR tendrá que decidir, como mínimo: una ventana de 90 días, la revocación, que las atestaciones recíprocas no cuentan, la identidad del atestador anclada a un dominio, y el consentimiento del atestador (Ley 1581).

### 5. Decaimiento y piso

Sin práctica, el Entrenamiento baja hacia un **piso**, a la velocidad que fija la volatilidad:

```
piso      = 0.4 × máximo histórico alcanzado en esa habilidad
valor(t)  = piso + (v₀ − piso) × 0.5 ^ (Δt / H)
```

- `v₀` es el valor justo después del último reto que sumó en esa habilidad, `Δt` son los días UTC enteros desde ese reto y `H` es la vida media de §2.
- "Sumó" quiere decir que el valor nuevo es mayor que el actual. Un reto que no suma no reinicia la curva, pero sí actualiza la **última práctica** que se muestra.
- **Un ejemplo:** programación agéntica (volátil) en 84 queda en 54 al año y en 42 a los 2 años. Kubernetes core (media) en 90 queda en 74 al año. Administración Linux (estable) en 92 queda en 85 al año.
- **Esperar a que decaiga nunca conviene:** el valor después de un reto crece con el valor de antes.
- **Pendiente para dojo-2:** ganar el piso completo practicando de forma espaciada (en 4 trimestres distintos), para no premiar las ráfagas.

### 6. Libro de eventos, aritmética exacta y recalculable

- **El Entrenamiento no se guarda: se calcula** a partir de un **libro de eventos** (`dojo_events`) con la fecha de cálculo explícita.
- **Solo entran las ejecuciones publicadas** de versiones públicas de un reto (ADR-002 §5). Lo privado no existe para el puntaje.
- Cada evento tiene:
  - el tipo;
  - la fecha (UTC);
  - el reto (id original y versión);
  - el nivel por habilidad;
  - la confianza y la prueba (commit, enlace a la corrida o a la attestation);
  - el hash del evento anterior, formando una **cadena de hashes**.

  La cabeza de la cadena se publica en cada actividad federada ([ADR-014](ADR-014-actividad.md)), para que reescribir el libro se note.
- **Retiro y derecho al olvido.** Retirar una ejecución deja una **tombstone** con el id del reto, la fecha y el hash, y se publica un `Delete`.
  - La tombstone **conserva la regla de "una vez por persona"**: el reto retirado no vuelve a sumar.
  - Retirar solo puede bajar el puntaje: quitar un evento nunca lo sube.
- **Aritmética:**
  - el estado se lleva en `float64` y solo se redondea al mostrar, hacia el entero más cercano (half away from zero);
  - Δt es en días UTC enteros;
  - los eventos del mismo día se ordenan por (fecha, hash).

  Las reglas viven en `internal/dojo` como implementación de referencia, con **test vectors** publicados (libro de entrada, fecha de cálculo, valor esperado), y `gremio dojo verify` recalcula un libro ajeno.
- **Las reglas tienen versión** (`rules: "dojo-1"`). Si cambian, todo se recalcula con la versión nueva, y la ficha dice con qué versión se calculó. Las otras instancias **recalculan con la fecha del día** y nunca guardan el número, porque un número congelado dejaría de decaer y sería engañoso.
- Este ADR **enmienda** dos anteriores:
  - **ADR-002 §5:** la ejecución publicada sí se expone hacia afuera como evento del libro;
  - **ADR-001 §4:** el inbox podrá recibir atestaciones cuando exista su ADR.

### 7. Lo que ve el reclutador y lo que ve el usuario

- **En la barra:**
  - el número;
  - la banda (N1–N4);
  - la confianza ("verificado por terceros", "reproducible" o "práctica propia");
  - el nombre de la habilidad del catálogo.

  Con texto, no solo con color.
- **En el tooltip:** la última práctica (mes y año), cuántos retos hay por confianza, el máximo con su fecha, y la volatilidad con su vida media.
- **En el detalle** (panel `:target`, sin JavaScript), un evento por línea: fecha, reto (enlace, versión, nivel, catálogo o autor), verificación (enlace) y cuánto aportó. Al pie: "Reglas dojo-1 · calculado el AAAA-MM-DD · descargar libro · cómo recalcular".
- **Texto neutral:** "sin práctica reciente", nunca "está perdiendo puntos". No hay animaciones de puntos, flechas rojas ni percentiles. La ficha dice que **mide resultados entregados**.
- **En el panel, para el usuario:**
  - por qué un reto no sumó ("ya superaste el techo de N2: necesitas un N3");
  - cuánto sumaría un reto antes de hacerlo;
  - que un reto propio cuenta como N2 como máximo, y cómo proponerlo al catálogo.

### 8. Quién propone los retos, quién fija su nivel y el catálogo revisado

- **Los retos los proponen personas o un modelo de IA** mediante los plugins. Si los genera una IA, nacen como **borrador** y una persona los revisa y publica (ADR-002 §4). **El nivel que propone la IA es solo una sugerencia** y no afecta el puntaje: un reto fuera del catálogo revisado cuenta como práctica propia (N2 como máximo).
- **El catálogo revisado de retos** vive en el repo de Gremio. Un reto entra por PR, revisado por **alguien distinto de su autor**, y el revisor queda visible. Cada reto del catálogo trae sus checks reproducibles.
- El mockup `docs/mockups/Retos.dc.html` describe un "agente revisor" que puntúa criterios con `[NN]/100`. **Eso queda fuera de alcance:** ninguna IA evalúa ni puntúa la solución.

### 9. Regulación y ética

- **El puntaje es una fórmula determinista definida por personas:** no es un sistema de IA que evalúa candidatos. Eso hay que preservarlo. Si un modelo de IA fijara niveles o revisara soluciones, Gremio quedaría en el Anexo III del AI Act (alto riesgo).
- **Veracidad (Ley 1581 art. 4 d; RGPD 5.1 d):** se federa el libro y no el número.
- **El decaimiento puede hacer visibles las pausas de carrera.** Por eso el texto es neutral, las fechas son aproximadas y lo declarado sigue siendo la señal principal. Ocultar la barra (todo o nada) queda para una versión posterior.

## Consecuencias positivas

- Dos señales separadas: lo que la persona dice que hizo, respaldado con logros, y lo que demostró hace poco, con su banda, su confianza y su fecha.
- Un número alto exige verificación fuerte: la autoverificación y los retos propios no pasan de 60.
- Las reglas son públicas, versionadas, deterministas y recalculables desde un libro encadenado. El orden de los mismos retos casi no cambia el resultado.
- La IA solo propone retos: no puntúa a nadie.

## Consecuencias negativas

- Mientras no exista el catálogo revisado de retos con checks reproducibles, el Entrenamiento no pasa de 60.
- Hay que mantener dos catálogos, el de habilidades y el de retos, revisados por terceros.
- La verificación por terceros depende de un CI con firma (GitHub o GitLab con Sigstore) o de instancias verificadoras.
- Los parámetros son hipótesis sin calibrar con datos reales. Se ajustarán con una versión nueva de las reglas, a partir de intentos compartidos de forma opt-in.

## Alternativas consideradas

| Alternativa | Por qué no |
|-------------|-----------|
| Multiplicadores de verificación (borrador anterior) | Cambian la velocidad, no el máximo: la evidencia débil llegaba a 99 |
| Ganancia por puntos fijos por nivel (borrador anterior) | Medía volumen y dependía del orden de los retos |
| Reemplazar lo declarado | Deja vacío cualquier perfil nuevo y borra una señal válida |
| Que la barra arranque igual a lo declarado (opción B) | Mezcla las dos fuentes |
| Que una IA califique la solución | Alto riesgo en el AI Act y opaco |
| Guardar el número | No se puede auditar y deja de decaer en otras instancias |

## Links relacionados

- [ADR-001](ADR-001-instancia-y-federacion.md) y [ADR-002](ADR-002-proyecto-y-ejecucion.md) (enmendados en §6), [ADR-010](ADR-010-ficha-de-jugador.md), [ADR-012](ADR-012-plugins.md), [ADR-014](ADR-014-actividad.md) y [ADR-015](ADR-015-medallas.md).
- gremio#119. La revisión independiente con sus simulaciones está resumida en la KB, en `tickets/personal/2026-11-07-gremio-119-dojo.md`.
