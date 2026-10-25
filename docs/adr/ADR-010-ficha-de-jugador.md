---
title: "Perfil como ficha de jugador: atributos puntuados y OVR por rol"
number: 10
date: 2026-10-25
domain: architecture
status: "aceptado"
tags: [profile, attributes, role-views, data-model, ui]
---

# ADR-010: Perfil como ficha de jugador: atributos puntuados y OVR por rol

## Contexto

El perfil público de v0.2 ([ADR-003](ADR-003-perfil-v1.md)) se lee como un CV: mucho texto, igual que LinkedIn. El dueño quiere que un reclutador entienda el rol de una persona en segundos, como pasa con la ficha de un jugador en FIFA: la posición (ST, LW…) y la forma de sus atributos dicen qué tipo de jugador es, y cada atributo se puede abrir para ver el detalle. El diseño se aprobó en Stitch el 2026-10-25 ([DESIGN.md](../design/DESIGN.md)).

Las skills de ADR-003 solo tienen un nivel de tres valores (`expert`, `solid`, `learning`), que no alcanza para dibujar un radar ni para calcular un índice por rol.

## Decisión

### 1. Atributos: ejes con sub-atributos puntuados

La extensión `gremio` suma `attributes`, una lista de **3 a 8 ejes**:

```json
{
  "code": "IAC",
  "name": "Infraestructura como código",
  "subs": [{ "name": "Terraform", "score": 94 }, { "name": "Go", "score": 45, "learning": true }],
  "skills": ["skill-iac"],
  "highlights": ["lulo-iac", "alkanza-iac"]
}
```

- **El código** tiene de 2 a 4 letras mayúsculas, es único y es la clave del atributo (anclas, pesos).
- **Cada sub-atributo** lleva un puntaje entero de **0 a 99**. Lo declara el dueño. `learning` lo marca como "aprendiendo", con la barra punteada.
- **El puntaje del eje es el promedio redondeado de sus sub-atributos.** No se declara aparte, para que el número grande nunca contradiga su desglose.
- `skills` y `highlights` referencian ids que existen. Son el respaldo del atributo: palabras clave y logros, y de los logros sale "dónde lo usó".

### 2. Puntajes autodeclarados, con su respaldo visible

Los puntajes son **autodeclarados** y la ficha lo dice al lado del número, junto con cuántos logros lo respaldan. No se calculan con una fórmula a partir de años, nivel y evidencia: una fórmula parece objetiva pero es igual de arbitraria, y es más difícil de explicar. El número es lo que el dueño afirma de sí mismo, y el valor está en que se puede contrastar con la evidencia que enlaza.

Las calificaciones de terceros (compañeros de proyecto, atestaciones firmadas) quedan para el módulo de equipos y no se mezclan en estos ejes.

### 3. OVR por rol: promedio ponderado

Cada vista suma `code` (la posición, de 2 a 4 letras: `PLE`, `SRE`…) y `weights`, un mapa de código de atributo a peso positivo. **El OVR de la vista es el promedio de los puntajes de los ejes ponderado por esos pesos.** Sin pesos, todos los ejes pesan igual. La primera vista es la posición principal.

Es el modelo de "un rol es una vista que combina capacidades con pesos": agregar un rol es agregar una vista, no rehacer el perfil.

### 4. Rasgos

`gremio.traits` es una lista corta de `{name, summary}`, que son los rasgos que se ven como insignias, como los PlayStyles de FIFA. Son texto del dueño, sin puntaje.

### 5. La página

- `/@usuario` muestra la ficha con la primera vista y `/@usuario/<slug>` la de otra vista. El selector son enlaces, sin JavaScript.
- **Radar y barras en SVG** renderizado en el servidor. La CSP prohíbe `style` inline, y los atributos de presentación de SVG no son CSS.
- **El detalle de un atributo** es un panel `:target` (`#atributo-iac`) que funciona sin JavaScript y se puede compartir como enlace.
- El CV completo (experiencia, estudios, certificados) sigue en la misma página, debajo de la ficha. La información no se pierde; solo deja de ser lo primero.

### 6. Lo que no se muestra

Siguen fuera, como en ADR-003, la edad, el género, la nacionalidad, la foto obligatoria y los salarios. La ficha tampoco muestra **años totales de experiencia**, porque dejan inferir la edad, ni **comparaciones entre personas** (percentiles, rankings): cada ficha se lee sola.

## Consecuencias positivas

- El rol se entiende de un vistazo: la forma del radar y la posición dicen más que un párrafo.
- Los números grandes se derivan de los pequeños y los OVR de los pesos, así que la ficha es consistente por construcción.
- Funciona sin JavaScript y con la CSP estricta.
- El JSON Resume exportado sin extensiones no cambia.

## Consecuencias negativas

- Un número autodeclarado puede inflarse. Se mitiga mostrando que es autodeclarado y cuántos logros lo respaldan.
- El dueño tiene que mantener más datos: puntajes, pesos y referencias.
- Un perfil sin `attributes` no tiene ficha: la página cae al encabezado y el CV de v0.2.

## Alternativas consideradas

- **Puntaje calculado** con nivel, años y cantidad de evidencia: descartado por lo explicado en §2.
- **Atributos y habilidades blandas en un mismo hexágono**: deforma la "posición" y mezcla fuentes distintas (autodeclarado y calificado por otros). Un segundo hexágono para las habilidades blandas quedó aparcado.
- **Detalle en una página aparte** (`/@usuario/atributo/iac`): funciona igual sin JavaScript, pero pierde el contexto de la ficha. El panel `:target` también es enlazable.

## Links relacionados

- [ADR-003](ADR-003-perfil-v1.md): modelo del perfil v1; este ADR lo extiende.
- [ADR-008](ADR-008-frontend.md): frontend renderizado en el servidor.
- gremio#36.
