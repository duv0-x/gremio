---
title: "Modelo Proyecto / Ejecución y cómo se federan los proyectos"
number: 2
date: 2026-10-24
domain: architecture
status: "aceptado"
tags: [projects, activitypub, json-ld, versioning, data-model]
---

# ADR-002: Modelo Proyecto / Ejecución y cómo se federan los proyectos

## Contexto

El módulo Proyectos permite escoger, crear y proponer proyectos para estudiar, mejorar habilidades o demostrarlas. Un proyecto lo puede proponer una persona o generarlo un modelo de IA, y cualquier usuario de otra instancia lo puede tomar. Hay que separar lo que se comparte (el enunciado) de lo que es de cada quien (su trabajo), y decidir cómo viaja un proyecto entre instancias.

[ADR-001](ADR-001-instancia-y-federacion.md) dejó fuera los proyectos como objetos ricos y limitó el inbox a follows; este ADR los define sin cambiar ese inbox.

## Decisión

### 1. Dos entidades

- **Proyecto**: el enunciado compartible. Tiene título, pitch, contexto de negocio, nivel (N1 Fundamentos → N4 Experto), disciplinas, stack, requisitos técnicos, restricciones, Definition of Done, entregables y duración estimada. Su estructura sale del mockup `docs/mockups/Retos.dc.html`.
- **Ejecución**: el trabajo de una persona sobre un proyecto. Contiene estado, enlaces a repos, demo, informe y notas privadas. Vive solo en la instancia de quien la hace.

Cualquier usuario crea proyectos en su instancia y puede ejecutar los propios o los ajenos.

### 2. El Proyecto es un objeto ActivityPub con extensión

- Se publica como `Article`: `name` (título), `summary` (pitch) y `content` (el enunciado completo renderizado en HTML). Así Mastodon y GoToSocial lo muestran legible sin entender la extensión.
- Los campos estructurados van en propiedades propias bajo un contexto JSON-LD del proyecto: `level`, `disciplines`, `stack`, `requirements`, `constraints`, `definitionOfDone`, `deliverables`, `estimatedDuration` (`xsd:duration`, soportado por `internal/goap/xsd`), `version` y `origin`.
- Al publicarlo se envía `Create` a los seguidores; con una versión nueva, `Update`; al borrarlo, `Delete` (queda un `Tombstone`).

### 3. Versionado por snapshot

- Cada edición publicada del enunciado crea una **versión nueva** (`version` entero creciente). Las versiones anteriores quedan accesibles e inmutables en `<id-del-proyecto>/versions/<n>`.
- Tomar un proyecto **copia el enunciado completo** en la instancia de quien lo toma y guarda `basedOn` (el `id` del proyecto) y la versión tomada.
- Si el autor publica una versión nueva, la instancia que tiene el snapshot lo detecta cuando le llega el `Update`, porque sigue al autor, o cuando vuelve a consultar el objeto. Se avisa al usuario y él decide si migra. La DoD de una ejecución nunca cambia sin su consentimiento.
- Si el autor borra el proyecto, los snapshots siguen siendo válidos.

### 4. Origen del proyecto

- `origin` vale `human` o `ai`. Si es `ai`, guarda también el modelo usado.
- Un proyecto generado por IA se crea como **borrador**: una persona lo revisa y lo publica, y queda como su autor (`attributedTo`). El proveedor de IA se decide en otro ADR.

### 5. La Ejecución

- **El código vive fuera**: la ejecución enlaza repos externos (GitHub, Codeberg, Forgejo…), una demo y un informe. La instancia guarda estado, enlaces, entregables y notas; no hospeda repos.
- Estados: `draft` → `in_progress` → `done`, o `abandoned` desde cualquiera de los anteriores.
- **Privada hasta que el usuario la publica.** Una ejecución privada no aparece en el perfil ni se federa. Al publicarla se ve en el perfil y, según ADR-001, se envía un `Note` público con enlace ("empezó / terminó el proyecto X").
- La ejecución no es un objeto rico de ActivityPub: hacia afuera solo existe su página pública y el `Note`.

### 6. Licencia del contenido de los proyectos

Tomar un proyecto copia texto ajeno a otra instancia, así que cada proyecto declara una licencia de contenido en `license`. **Por defecto es CC BY 4.0**: se puede copiar y adaptar citando al autor; `attributedTo` y `basedOn` cubren esa atribución.

## Consecuencias positivas

- Mastodon y GoToSocial ven los proyectos como artículos legibles, y las instancias de la plataforma los entienden completos.
- El snapshot protege a quien ejecuta: nadie le mueve los criterios, y el trabajo sobrevive aunque el autor borre o cambie el proyecto.
- No hospedar código evita construir un forge y deja los repos donde la gente ya trabaja.
- Privada por defecto: estudiar no expone trabajo a medias.

## Consecuencias negativas

- Hay que mantener un contexto JSON-LD propio en una URL **estable para siempre**.
- Con el inbox limitado a follows, el autor no se entera de quién tomó su proyecto ni recibe las versiones nuevas de un `Update` si no sigue al autor; el aviso de versión depende de consultar el objeto de nuevo.
- Copiar enunciados duplica datos entre instancias.
- Enlazar repos externos significa que la evidencia puede cambiar o desaparecer después de terminar. Fijar el commit es tema del ADR de evaluación.

## Alternativas consideradas

| Alternativa | Razón de descarte |
|-------------|-------------------|
| Importar proyectos por URL, sin federarlos | No aparecen en timelines; se pierde descubrimiento |
| Proyectos solo locales en v1 | Un proyecto propuesto por otra persona no se podría tomar |
| Referencia viva al enunciado | El autor podría cambiar la DoD después de que terminaste |
| Git alojado en la instancia | Construir un mini-forge; fuera del foco |
| Ejecución pública por defecto | Expone trabajo de estudio a medias |
| Tipo AP propio sin `Article` | Mastodon y GoToSocial no lo mostrarían |

## Pendiente de resolver

- **URL del contexto JSON-LD.** Debe ser estable para siempre y depende del nombre y del dominio del proyecto; hay que decidirla antes de federar el primer proyecto.
- Validar en el spike de ADR-001 que Mastodon y GoToSocial muestran bien un `Article` con propiedades extra. Evaluar también `type: ["Article", "Project"]` como alternativa.
- Si en el futuro el autor debe enterarse de quién tomó su proyecto, hay que ampliar el inbox (otro ADR).

## Links relacionados

- [ADR-001](ADR-001-instancia-y-federacion.md): modelo de instancia y federación.
- Mockup base: `docs/mockups/Retos.dc.html`.
- Próximos ADRs: modelo de perfil v1, licencia del software, proveedor de IA, evaluación de ejecuciones.
