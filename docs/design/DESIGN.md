# Gremio — sistema de diseño

Fuente de los tokens y principios visuales de Gremio. El código ([ADR-008](../adr/ADR-008-frontend.md)) los lee de aquí y los define como variables del tema de Tailwind. El diseño visual vive en Google Stitch, proyecto **Gremio** (`projects/15030107499092180016`), cuyo `designMd` es un resumen de este archivo. Stitch no guarda la fuente de etiquetas (la deja en Public Sans), la escala tipográfica ni el espaciado del tema, y deriva su propia paleta de los colores semilla (p. ej. primario `#066A5E`, fondo `#F9FAF7`): lo que vale son los valores de este archivo, que también lleva el `designMd`. Si algo cambia, se cambia aquí primero.

## Principios

- **Tono**: sobrio y editorial, de taller. Un perfil profesional que se lee rápido: mucha tinta sobre papel, un solo acento.
- **Un acento**: verde gremio `#0F6E61` para lo accionable (enlaces, botón primario, foco del selector de vistas, estado seleccionado). El resto es neutro.
- **No se usa**: degradados, fotos de stock, ilustraciones decorativas, glassmorphism, sombras de colores, emojis como iconos, ni texto en mayúsculas en párrafos.
- **Datos en mono**: fechas, versiones, métricas, handles (`@root@duv0-x.cloud`) y etiquetas pequeñas van en mono.
- **Nada personal que discrimine**: ninguna pantalla muestra edad, género, nacionalidad, foto obligatoria ni salarios ([ADR-003](../adr/ADR-003-perfil-v1.md)).
- **Accesibilidad**: contraste AA (texto ≥ 4.5:1, texto grande y bordes de controles ≥ 3:1), foco siempre visible, objetivos táctiles de **44 px** mínimo, HTML semántico y funcionamiento sin JavaScript en el perfil público.
- **Claro y oscuro**: el modo sigue `prefers-color-scheme`. Stitch admite un solo modo por design system: el del proyecto es el claro y el oscuro se define en la tabla de abajo.

## Color

| Token | Claro | Oscuro | Uso |
|-------|-------|--------|-----|
| `bg` | `#F3F4F1` | `#111315` | Fondo de página (papel) |
| `surface` | `#FFFFFF` | `#1A1D20` | Tarjetas, inputs, tablas |
| `surface-2` | `#ECEDE9` | `#22262A` | Hover de filas, bloques secundarios |
| `ink` | `#16181B` | `#ECEDE9` | Texto principal, títulos |
| `muted` | `#4D525A` | `#A9AEB5` | Texto secundario, lead |
| `faint` | `#6B7078` | `#878C94` | Meta, ayudas, footer |
| `line` | `#DADCD6` | `#2E3237` | Bordes y separadores decorativos |
| `control` | `#7A8088` | `#767B83` | Borde de inputs y checkboxes (≥ 3:1) |
| `accent` | `#0F6E61` | `#4FB8A6` | Enlaces, botón primario, seleccionado |
| `accent-hover` | `#0A4F46` | `#7CCDBF` | Hover/pressed del acento |
| `accent-soft` | `#DDEFEA` | `#16312C` | Fondo de chip/aviso de acento, fila seleccionada |
| `on-accent` | `#FFFFFF` | `#0B1F1C` | Texto sobre `accent` |
| `success` | `#1E6B34` | `#6CC287` | Éxito |
| `warning` | `#9A4A06` | `#E0A256` | Aviso |
| `danger` | `#B42318` | `#F08A7E` | Error, botón de peligro |
| `focus` | `#1D4ED8` | `#8AB4F8` | Anillo de foco (2 px + 2 px de separación) |

- Los estados (`success`, `warning`, `danger`) usan un fondo suave al 10–12 % de su color, con el texto en el color pleno.
- La sección invertida del perfil (selector de vistas) usa `ink` de fondo y `bg` de texto en claro; en oscuro usa `surface-2`.
- Cambio respecto a los mockups: el gris `#7A8088` solo daba 3.6:1 sobre `bg`, así que como texto pasa a `faint` `#6B7078`; `#7A8088` queda para bordes de controles y barras de nivel.

### Contraste verificado (WCAG 2.2)

| Par | Claro | Oscuro |
|-----|-------|--------|
| `ink` / `bg` | 16.11 | 15.83 |
| `muted` / `bg` | 7.12 | 8.34 |
| `faint` / `bg` | 4.51 | 5.51 |
| `accent` / `bg` | 5.56 | 7.75 |
| `accent` / `surface` | 6.13 | 7.04 |
| `on-accent` / `accent` | 6.13 | 7.12 |
| `accent-hover` / `accent-soft` | 7.93 | — |
| `accent` / `accent-soft` | — | 5.79 |
| `success` / `bg` | 5.93 | 8.60 |
| `warning` / `bg` | 5.67 | 8.40 |
| `danger` / `bg` | 5.95 | 7.66 |
| `focus` / `bg` | 6.07 | 8.84 |
| `control` / `surface` | 3.98 | 3.98 |

## Tipografía

| Familia | Uso | En Stitch |
|---------|-----|-----------|
| **Space Grotesk** | Títulos, nombre del perfil, cifras grandes | Space Grotesk |
| **IBM Plex Sans** | Texto, botones, formularios | IBM Plex Sans |
| **IBM Plex Mono** | Datos, etiquetas pequeñas, código | **JetBrains Mono** (Stitch no tiene IBM Plex Mono) |

El código embebe y sirve las tres familias de IBM/Google desde la instancia (sin Google Fonts). JetBrains Mono solo existe en Stitch.

| Nivel | Familia | Tamaño / interlineado | Peso | Tracking |
|-------|---------|-----------------------|------|----------|
| `display-lg` | Space Grotesk | 64 / 1.02 (móvil 40) | 700 | -0.02em |
| `headline-lg` | Space Grotesk | 36 / 1.15 (móvil 28) | 700 | -0.01em |
| `headline-md` | Space Grotesk | 28 / 1.2 | 700 | -0.01em |
| `title-md` | Space Grotesk | 20 / 1.3 | 600 | 0 |
| `body-lg` | IBM Plex Sans | 18 / 1.6 | 400 | 0 |
| `body-md` | IBM Plex Sans | 16 / 1.6 | 400 | 0 |
| `body-sm` | IBM Plex Sans | 14 / 1.5 | 400 | 0 |
| `label-md` | IBM Plex Sans | 15 / 1.2 | 600 | 0 |
| `eyebrow` | IBM Plex Mono | 13 / 1.4, mayúsculas | 500 | 0.08em |
| `data-md` | IBM Plex Mono | 14 / 1.5 | 400 | 0 |

## Forma

- **Radios**: `sm` 4 px (chips de dato, código), `md` 8 px (botones, inputs), `lg` 14 px (tarjetas, paneles), `full` (pills: tabs de vistas, chips de skill, avatar).
- **Bordes**: 1 px `line` en tarjetas y separadores; 1.5 px `control` en inputs; 1.5 px en tabs no seleccionadas.
- **Sombras**: ninguna. La jerarquía sale del borde y del contraste `surface` sobre `bg`. Única excepción: menús flotantes, con `0 8px 24px rgb(22 24 27 / 0.12)`.
- **Iconos**: trazo de 1.5 px, 20 px, sin relleno; siempre con texto o `aria-label`.

## Espacio y layout

- Escala de 4 px: `1`=4, `2`=8, `3`=12, `4`=16, `5`=20, `6`=24, `8`=32, `10`=40, `12`=48, `14`=56, `18`=72.
- **Contenedor**: 1200 px máximo, gutter lateral de 24 px (16 px en móvil).
- **Columna de lectura** (resúmenes, logros largos): 720 px máximo.
- **Secciones**: 56–72 px de padding vertical en desktop, 40 px en móvil.
- **Grid**: tarjetas en columnas fluidas con gap de 16–24 px; una sola columna por debajo de 640 px.
- **Objetivo táctil**: todo control mide al menos 44 × 44 px.
- **Impresión** (`/cv`): A4/Letter, fondo blanco, sin nav ni footer, solo `ink`, `muted` y `line`; el acento se imprime como `ink`.
