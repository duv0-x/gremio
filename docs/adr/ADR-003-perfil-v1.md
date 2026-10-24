---
title: "Modelo del perfil v1: JSON Resume con evidencia y vistas por rol"
number: 3
date: 2026-10-24
domain: architecture
status: "aceptado"
tags: [profile, json-resume, role-views, data-model, cv]
---

# ADR-003: Modelo del perfil v1: JSON Resume con evidencia y vistas por rol

## Contexto

El perfil es lo que ve cualquier reclutador o persona en internet. Tiene que ser portable (sin lock-in), mostrar evidencia real en vez de solo afirmaciones, y adaptarse al rol que busca quien lo mira. La idea de modelar todo por capacidades con pesos (ver la KB, `ideas-aparcadas.md`) es más potente pero exige mucho diseño; el v1 necesita algo que se pueda construir ya y evolucionar.

[ADR-002](ADR-002-proyecto-y-ejecucion.md) define las ejecuciones de proyectos, que son la evidencia natural del perfil.

## Decisión

### 1. Base: el esquema de JSON Resume

- El perfil usa las secciones de [JSON Resume](https://jsonresume.org/schema) (v1.0.0): `basics`, `work`, `education`, `skills`, `languages`, `certificates`, `awards`, `publications`, `volunteer`, `interests` y `projects`.
- Cada elemento de una lista (un trabajo, un skill, un logro) lleva un **`id` interno estable**. JSON Resume no lo tiene, y las vistas y la evidencia lo necesitan para referenciar elementos.
- Nivel de skill: `expert` / `solid` / `learning` (Experto / Sólido / Aprendiendo del mockup `Main`).
- **El modelo no tiene campos de fecha de nacimiento, género ni nacionalidad.** La foto (`basics.image`) es opcional.

### 2. Evidencia enlazada

- Un skill o un logro (`work[].highlights`) puede tener `evidence`: una lista de referencias a **ejecuciones publicadas** de la propia instancia o a URLs externas (repo, charla, artículo).
- En el perfil público, un skill con evidencia muestra sus enlaces; uno sin evidencia es una afirmación, y se ve distinto.
- Las ejecuciones publicadas se exportan además en la sección `projects` de JSON Resume.

### 3. Vistas por rol (simples, sin salarios)

- Una **vista** tiene nombre (p. ej. "Platform Engineer"), `slug` y URL propia (`/@usuario/<slug>`), y define:
  - un resumen propio, que reemplaza a `basics.summary`;
  - qué skills se destacan y en qué orden;
  - qué trabajos y logros se muestran;
  - qué ejecuciones se destacan.
- Una vista es un **filtro sobre el mismo perfil**, no una copia: editar un trabajo lo actualiza en todas las vistas.
- La vista por defecto es el perfil completo. El selector de rol del mockup `Main` cambia entre vistas.
- Los rangos salariales por vista **no** entran en el v1.
- Las vistas son páginas web, no objetos ActivityPub: el Actor `Person` (ADR-001) siempre refleja la vista por defecto.

### 4. Visibilidad de datos de contacto

- `basics.email` y `basics.phone` son **privados por defecto**: no se muestran en el perfil, en el Actor ni en el CV público, salvo que el usuario los habilite. Es la misma lección del CV sanitizado de `me.duv0-x.cloud`: el teléfono no se publica.

### 5. Edición: panel web + importar/exportar

- El perfil se edita con formularios en el panel de configuración.
- **Importar** JSON Resume o YAML con el mismo esquema: se muestra una vista previa y el usuario confirma; reemplaza el perfil (no lo mezcla).
- **Exportar** a JSON Resume válido. Los campos propios (`id`, `evidence`, vistas) se exportan aparte o como extensión, para no romper a quien consume el JSON estándar.
- Cada cambio guardado dispara el `Update` del `Person` (ADR-001).

### 6. CV: hoja de impresión

- `/cv` (y `/@usuario/<slug>/cv` por vista) renderiza el perfil con CSS de impresión. "Guardar como PDF" en el navegador produce el CV.
- No hay librería de PDF ni navegador headless en el binario.

## Consecuencias positivas

- Portabilidad real: entra y sale JSON Resume, compatible con su ecosistema de temas y con herramientas como RenderCV.
- La evidencia conecta los dos módulos: terminar un proyecto mejora el perfil.
- Las vistas cubren el multi-rol sin duplicar datos y sin el diseño completo de capacidades.
- El CV por impresión es gratis en código y sale igual que la web.
- Sin datos de edad, género ni nacionalidad en el modelo, la regla de equidad del armador de equipos ya está respaldada desde la base.

## Consecuencias negativas

- JSON Resume modela títulos y experiencia, no capacidades. Migrar al modelo de capacidades más adelante requerirá otro ADR y una migración de datos.
- El PDF depende del navegador de quien lo genera; el resultado puede variar un poco entre navegadores.
- Importar reemplaza el perfil completo: no hay merge.
- Las vistas no se federan: desde Mastodon solo se ve el perfil por defecto.

## Alternativas consideradas

| Alternativa | Razón de descarte |
|-------------|-------------------|
| Capacidades con pesos desde el v1 | Mucho diseño antes de construir; se deja como evolución |
| JSON Resume puro | Sin evidencia: el perfil queda en afirmaciones |
| Un solo perfil sin vistas | El multi-rol es parte central de la propuesta (mockup `Main`) |
| Vistas con salarios | Riesgos de negociación y privacidad aún sin resolver (KB, `ideas-aparcadas.md`) |
| Archivo YAML como fuente de verdad | Cómodo para devs, excluye al resto |
| Panel + archivo con sincronización | Resolver conflictos entre las dos fuentes |
| PDF generado en el servidor | Librería de PDF o navegador headless dentro de un binario único |

## Pendiente de resolver

- **Perfil en varios idiomas** (ES/EN). JSON Resume no lo soporta: ¿un perfil por idioma, o campos traducibles?
- Verificar que el validador oficial de JSON Resume acepte las extensiones, o exportarlas en un archivo aparte.
- Dónde se guarda el perfil (documento JSON vs. tablas): ADR de almacenamiento.

## Links relacionados

- [ADR-001](ADR-001-instancia-y-federacion.md): el Actor `Person` y su `Update`.
- [ADR-002](ADR-002-proyecto-y-ejecucion.md): las ejecuciones como evidencia.
- Mockup: `docs/mockups/Main.dc.html`.
