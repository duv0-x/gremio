---
title: "Frontend: templ + htmx renderizado en el servidor, estilos con Tailwind"
number: 8
date: 2026-10-24
domain: architecture
status: "aceptado"
tags: [frontend, templ, htmx, tailwind, ssr, accessibility]
---

# ADR-008: Frontend: templ + htmx renderizado en el servidor, estilos con Tailwind

## Contexto

Gremio tiene dos caras web: el **perfil público** (más sus vistas por rol, `/cv` y las páginas de proyectos), que lee cualquier reclutador o persona en internet, y el **panel** de configuración, que usa solo el dueño. El perfil público tiene que cargar rápido, verse completo sin JavaScript y ser indexable por buscadores. Todo se embebe en el binario ([ADR-007](ADR-007-lenguaje-y-distribucion.md)) y el proyecto evita toolchains que no sean Go.

## Decisión

### 1. HTML renderizado en el servidor con templ

- Todas las páginas, públicas y del panel, se renderizan en el servidor con **[templ](https://templ.guide)** (MIT): componentes Go tipados que se compilan a código Go y se verifican al compilar, igual que sqlc con el SQL.
- El código generado (`*_templ.go`) se commitea ([ADR-007](ADR-007-lenguaje-y-distribucion.md)). La versión de templ se fija con la directiva `tool` del `go.mod` (`go tool templ generate`).
- Las mismas rutas responden **HTML o ActivityPub según el `Accept`**: `/users/<x>` entrega la página del perfil a un navegador y el `Person` a otra instancia.

### 2. Interacción con htmx

- **[htmx](https://htmx.org)** (0BSD) para la interacción del panel (formularios, edición en línea, vistas previas) sin escribir JavaScript a mano. Se usa la versión 4.
- htmx se **embebe y se sirve desde la propia instancia**, sin CDN: una instancia self-hosted no depende de terceros para funcionar ni filtra visitas.
- **El perfil público funciona sin JavaScript.** htmx solo mejora; los formularios del panel también funcionan con un submit normal.

### 3. Estilos con Tailwind (CLI standalone)

- **Tailwind CSS v4** (MIT) con su **binario standalone**, sin Node. Lee las plantillas templ y genera un solo CSS, que se commitea y se embebe.
- La versión del binario se fija en el repo y se descarga verificando su checksum.
- Los **tokens del sistema visual** de los mockups se definen como variables del tema de Tailwind:
  - fondo `#F3F4F1`, tinta `#16181B`, acento `#0F6E61`;
  - tipografías Space Grotesk (títulos), IBM Plex Sans (texto) e IBM Plex Mono (datos).
- Las tipografías se embeben y se sirven desde la instancia (sin Google Fonts), por la misma razón que htmx.
- Modo oscuro con `prefers-color-scheme`.

### 4. Accesibilidad

HTML semántico, contraste AA y navegación completa por teclado, desde el primer componente. Los mockups ya usan `role`, `aria-*` y objetivos táctiles de 44 px.

## Consecuencias positivas

- Un solo lenguaje y un solo modelo mental: el servidor Go produce el HTML; no hay una API JSON aparte para el frontend.
- El perfil público carga rápido, es indexable y funciona sin JS: justo lo que necesita un reclutador.
- templ detecta errores de plantilla al compilar.
- Sin CDNs ni fuentes externas: privacidad para quien visita y ninguna dependencia de terceros en runtime.

## Consecuencias negativas

- Dos generadores más en el flujo (templ y el CLI de Tailwind) además de sqlc.
- Interacciones muy ricas (p. ej. el armador de equipos tipo FIFA de la fase siguiente) pueden pedir JavaScript propio o islas interactivas; se decidirá cuando lleguen.
- htmx 4 es una versión mayor reciente (2026-08): menos ejemplos y comunidad que la 2.
- Las clases de Tailwind alargan el markup de los componentes.

## Alternativas consideradas

| Alternativa | Razón de descarte |
|-------------|-------------------|
| `html/template` + htmx | Errores de plantilla en ejecución, no al compilar |
| SPA embebida (Svelte/React) | Toolchain de Node, API JSON completa y SSR extra para el SEO del perfil |
| Híbrido (perfil SSR + panel SPA) | Dos stacks de frontend que mantener |
| CSS propio con tokens | Más CSS a mano; se eligió iterar más rápido con utilidades |
| Framework classless (Pico.css) | Menos control fino sobre el diseño de los mockups |
| htmx 2 | Rama anterior; se empieza directo en la versión mayor vigente |

## Links relacionados

- [ADR-003](ADR-003-perfil-v1.md): perfil, vistas por rol y `/cv` con CSS de impresión.
- [ADR-007](ADR-007-lenguaje-y-distribucion.md): código generado commiteado y assets embebidos.
- Mockups: `docs/mockups/Main.dc.html`, `docs/mockups/Retos.dc.html`.
- Pendiente: autenticación del dueño en el panel (ADR-009).
