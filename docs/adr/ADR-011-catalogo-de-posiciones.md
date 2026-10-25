---
title: "Catálogo común de posiciones y seniority de tres niveles"
number: 11
date: 2026-10-25
domain: architecture
status: "aceptado"
tags: [profile, positions, seniority, role-views, interoperability]
---

# ADR-011: Catálogo común de posiciones y seniority de tres niveles

## Contexto

En [ADR-010](ADR-010-ficha-de-jugador.md) cada vista lleva un código de posición (`PLE`, `SRE`…) que elige el dueño. Así cada instancia inventa los suyos, y el armador de equipos y la búsqueda entre instancias no pueden saber si dos "BE" son lo mismo. Además, el perfil del dueño mezclaba posición y seniority: "DevOps Lead" (`DOL`) es DevOps con nivel de líder.

## Decisión

1. **Gremio trae un catálogo común de posiciones** (`internal/profile/positions.go`). Cada una tiene código, nombre, familia y una descripción de una línea, y lo cubre todo, desde diseño (UX, UI) hasta IA (AI Engineer, Agentic Developer, AI Evals). El catálogo es parte del contrato entre instancias, igual que el contexto JSON-LD ([ADR-005](ADR-005-nombre-y-contexto-json-ld.md)): **un código publicado no cambia ni se reutiliza**, y las posiciones nuevas entran por PR.
2. **`views[].code` apunta a una posición del catálogo.** La página toma de ahí el nombre y la explicación del tooltip. Un código propio se acepta, pero se muestra como posición fuera del catálogo.
3. **El catálogo solo tiene posiciones, no niveles.** El seniority es `views[].level`, con tres valores: `jr` (Junior), `semi-sr` (Semi Senior) y `sr` (Senior). "Lead" o "Staff" no son niveles: el liderazgo técnico tiene sus propias posiciones (`TL`, `EM`, `STF`).
4. **Los atributos siguen siendo libres por persona.** Un diseñador no tiene `IAC`, así que no hay seis ejes iguales para todos. Cada perfil declara los que tenga (al menos 3), **el radar muestra los 6 de mayor puntaje** y la grilla los muestra todos. Esto reemplaza el límite de 3 a 8 ejes de ADR-010 §1.

## Consecuencias

- Las siglas se explican solas con el catálogo, y dos instancias pueden comparar posiciones.
- Hay que mantener el catálogo: los títulos del mercado cambian rápido. Se agregan posiciones; no se borran ni se renombran códigos.
- Para comparar atributos entre personas haría falta un catálogo de atributos por familia. Queda para el armador de equipos.
