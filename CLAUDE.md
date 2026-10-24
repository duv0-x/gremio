# CLAUDE.md — reverse-mini-ats

El dueño es Duván Ballén (Senior Platform Engineer, aprendiendo Go). Comunícate en español, con términos técnicos en inglés.

## Estado: fase de decisiones

No hay código. Antes de construir, cada decisión de diseño, arquitectura o stack se documenta como ADR en `docs/adr/ADR-NNN-titulo.md`. No escribas código ni crees issues hasta que Duván lo pida.

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

## Arquitectura tentativa (sin ADR todavía)

Go, un binario con frontend embebido (`embed`), SQLite por defecto y Postgres opcional, Docker + Helm. Código idiomático y explicado: Duván aprende Go con este proyecto. Referencia local de Go + ActivityPub: el fork de GoToSocial en `/Users/lx-duv0-x/Documents/Repositories/echo.duv0-x.cloud` (AGPL: no copiar código sin decidir antes la licencia).

## Decisiones pendientes (candidatas a ADR)

1. Modelo de instancia y qué se federa por ActivityPub.
2. Modelo Proyecto (enunciado compartido) vs. Ejecución (trabajo de cada usuario).
3. Modelo de perfil para el v1: capacidades con evidencia o algo cercano a JSON Resume.
4. Licencia: AGPL (inclinación) vs. MIT.

## Mockups

`docs/mockups/*.dc.html`: markup en `<x-dc>` con `{{holes}}`, `<sc-for>`, `<sc-if>` y una clase `Component extends DCLogic`. Úsalos como especificación de layout y contenido. `Main` (perfil) y `Retos` (base del módulo Proyectos) aplican al alcance actual; `Pipeline` y `Equipos` reflejan ideas aparcadas.
