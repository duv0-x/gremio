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

## Componentes

Pantallas de referencia en Stitch: "Componentes · Gremio Design System" (claro, `screens/5bfea9db99a54704a835eaa2a29f4d5e`) y "Catálogo de Componentes — modo oscuro" (`screens/8c82eaf683814e8591441ef9eb9718f9`). Capturas: [claro](stitch/componentes-claro.png) y [oscuro](stitch/componentes-oscuro.png). Los textos de ejemplo de Stitch son de relleno; los reales salen del perfil. Todos los interactivos tienen foco visible (`focus`, 2 px con 2 px de separación) y miden al menos 44 × 44 px.

### Acciones

| Componente | Anatomía | Estados | Cuándo |
|------------|----------|---------|--------|
| Botón primario | 44 px de alto, padding 0 18 px, radio `md`, `label-md`; fondo `accent`, texto `on-accent` | hover `accent-hover`; foco; deshabilitado al 40 % de opacidad y `cursor: not-allowed` | Una sola acción principal por vista ("Enviarme un enlace", "Guardar") |
| Botón secundario | Igual, fondo transparente, borde 1.5 px `ink`, texto `ink` | hover fondo `surface-2` | Acciones alternativas ("Exportar JSON Resume") |
| Botón ghost | Sin borde ni fondo, texto `ink` | hover fondo `surface-2` | Acciones de baja jerarquía en barras y tablas |
| Botón de peligro | Fondo `danger`; texto blanco en claro (6.57:1) y `bg` en oscuro (7.66:1) | hover 10 % más oscuro | Acciones destructivas ("Cerrar todas las sesiones"); siempre con confirmación |
| Enlace | Texto `accent`, subrayado con offset 3 px | hover `accent-hover` | Navegación y referencias; nunca un botón disfrazado |

Un botón con icono solo lleva `aria-label`. Con htmx, el botón pasa a `aria-busy="true"` y muestra el spinner mientras dura la petición.

### Formularios

- **Etiqueta** siempre visible encima (`label-md`), nunca solo placeholder. **Ayuda** debajo en `body-sm` `faint`, enlazada con `aria-describedby`.
- **Input, textarea, select**: alto 44 px (textarea desde 96 px), fondo `surface`, borde 1.5 px `control`, radio `md`, padding 0 12 px.
  - Foco: anillo `focus`.
  - Error: borde `danger`, mensaje debajo en `danger` con icono, `aria-invalid="true"`; el mensaje dice cómo arreglarlo.
  - Deshabilitado: fondo `surface-2`, texto `faint`.
- **Checkbox**: caja de 20 px (borde 1.5 px `control`, radio `sm`) dentro de un área clicable de 44 px que incluye la etiqueta; marcado en `accent`.

### Perfil

- **Chip de skill**: pill (`full`), `body-sm` 600, alto 32 px (área de 44 px si es enlace). El nivel ([ADR-003](../adr/ADR-003-perfil-v1.md)) se ve en una barra de 6 px y en el texto, nunca solo por color:
  - `expert` "Experto": barra llena `ink`;
  - `solid` "Sólido": barra al 70 % `control`;
  - `learning` "Aprendiendo": barra al 35 % con borde punteado `control`.
  - **Con evidencia** enlazada: fondo `accent-soft`, texto `accent-hover`, icono de enlace; es un `<a>` a la evidencia.
  - **Sin evidencia**: fondo transparente, borde 1 px `line`, texto `ink`; no es interactivo.
- **Tarjeta de experiencia**: `surface`, borde `line`, radio `lg`, padding 24 px. Cargo (`title-md`), organización (`body-md` `muted`), fechas en `data-md` (`2022-03 → hoy`), logros en lista con viñeta `accent` y skills relacionadas en chips al pie.
- **Tarjeta de ejecución**: título (`title-md`) que enlaza al proyecto, estado en chip de dato (mono, radio `sm`), fecha, rol propio y stack en chips de dato. Una ejecución sin resultado publicado no se muestra en el perfil público.
- **Selector de vistas por rol**: `role="tablist"` con tabs pill de 44 px sobre la banda invertida. Seleccionada: fondo `bg`, texto `ink`; resto: transparente, borde 1.5 px `#767B83` (el `#4A4F56` del mockup daba 2.15:1; este da 4.18:1 en claro y 3.58:1 en oscuro), texto `bg`. Sin JavaScript cada tab es un enlace a `/@usuario/<vista>`; con htmx cambia el contenido sin recargar y actualiza la URL.

### Estructura

- **Nav**: alto 64 px, fondo `bg`, borde inferior `line`. A la izquierda el nombre y el handle en `data-md`; a la derecha los enlaces de sección y, solo con sesión, "Panel".
- **Footer**: borde superior `line`, `body-sm` `faint`: `Gremio v<versión> · AGPL-3.0 · Source`. **El enlace "Source" al código de la versión desplegada es obligatorio** ([ADR-004](../adr/ADR-004-licencia.md)).
- **Contenedor**: 1200 px y gutter de 24/16 px; la columna de lectura de 720 px para texto corrido.
- **Tabla**: encabezado en `eyebrow`, filas de 48 px con borde inferior `line`, hover `surface-2`, números alineados a la derecha en `data-md`. Por debajo de 640 px pasa a tarjetas apiladas.
- **Alerta**: radio `md`, padding 12 16 px, borde izquierdo de 3 px y fondo suave del color de estado (`success`, `warning`, `danger`, `accent` para info), icono más texto. `role="status"` para éxito e info, `role="alert"` para error.

### Estados

- **Vacío**: icono de trazo, una frase de qué falta y una acción ("Aún no hay ejecuciones publicadas" + "Publicar una").
- **Error**: alerta `danger` con lo que pasó, en palabras del usuario, y un botón "Reintentar".
- **Cargando**: esqueletos `surface-2` con la forma del contenido (sin animación si `prefers-reduced-motion`) y spinner de 16 px en botones. Las regiones que actualiza htmx llevan `aria-live="polite"`.

### Revisión de accesibilidad (pantallas de Stitch)

- **Contraste**: todos los pares de texto y de controles cumplen AA en ambos modos (tablas de arriba). El borde de las tabs no seleccionadas pasa de `#4A4F56` (2.15:1) a `#767B83`.
- **Foco**: visible en botones e inputs de los dos modos (anillo `focus`).
- **Objetivos de 44 px**: Stitch dibuja las tabs de vistas y los chips de skill más bajos (unos 32 px). En el código las tabs miden 44 px y los chips enlazados tienen un área clicable de 44 px; la pantalla de Stitch no es la referencia de tamaño.
- **Nivel de skill**: Stitch lo marca con un punto lleno, medio o vacío. El código usa la barra de 6 px y el texto del nivel, para no depender de una forma pequeña.

## Pantallas del módulo Perfil

Diseño vigente del módulo Perfil, en el proyecto de Stitch. Reemplaza a `docs/mockups/Main.dc.html`. Las capturas están en `stitch/`.

| Pantalla | Stitch (`screens/…`) | Captura |
|----------|----------------------|---------|
| **Ficha de jugador, desktop** (oscuro) | `50ca36da74d642fbb1acfc1c86344c8c` | [ficha-desktop](stitch/ficha-desktop.png) |
| **Ficha de jugador, móvil** (claro) | `fea526eaf1744082b58543e37e21d158` | [ficha-movil](stitch/ficha-movil.png) |
| **Detalle de un atributo** | `72b1e5aaa4fb49a7b346d7273b48d99b` | [ficha-detalle](stitch/ficha-detalle.png) |
| Perfil público, desktop (v0.2, reemplazado por la ficha) | `39d0ba8ef5124247b45542cec6023929` | [perfil-desktop](stitch/perfil-desktop.png) |
| Perfil público, móvil (v0.2, reemplazado por la ficha) | `e44bc3bf5f4c417dacf6507649173816` | [perfil-movil](stitch/perfil-movil.png) |
| Vista por rol (v0.2, reemplazado por la ficha) | `e564cf3a744a41c5a420f2a70310ab05` | [vista-rol](stitch/vista-rol.png) |
| `/cv` (A4, impresión) | `d0a0c30627964ef4a647754d29cdf362` | [cv](stitch/cv.png) |
| Login: entrar, revisa tu correo, confirmar, enlace vencido | `4b384e5fa5494c8eb360dd2dc812940f` | [login](stitch/login.png) |
| Panel: edición del perfil | `cf3d9637e43a4bf6942f23d66f11dc17` | [panel-perfil](stitch/panel-perfil.png) |
| Panel: vistas por rol | `fd4348a7318f496e961864e64413cda5` | [panel-vistas](stitch/panel-vistas.png) |
| Panel: importar y exportar JSON Resume | `0e83107843db49d9932010a55be428d1` | [panel-importar](stitch/panel-importar.png) |
| Panel: sesiones | `78e23caffdc349008dda7ff2ada3a365` | [panel-sesiones](stitch/panel-sesiones.png) |
| Login y panel, móvil | `c5073892ba214bc692e349c763d80ec7` | [login-panel-movil](stitch/login-panel-movil.png) |

Notas para implementar:

- **Ficha de jugador** ([ADR-010](../adr/ADR-010-ficha-de-jugador.md)), aprobada el 2026-10-25:
  - **No vale el contenido de Stitch**: los puntajes, las certificaciones CKA y HashiCorp, "14+ años", "Percentil 98.4 / top 2 %", "-23 % costo AWS", "Disponible" ni "Técnica verificada". Sí vale el contenido del perfil importado.
  - **Sin años totales ni comparaciones** entre personas.
  - **Detalle sin JavaScript**: el panel es un `<section>` con `:target`, sin aria-modal (no atrapa el foco).
  - **Insignias de rasgos**: son la única excepción al acento único. Llevan un hexágono del color del rasgo con un brillo arriba y un círculo blanco con un ícono de [Lucide](https://lucide.dev) (ISC, embebido en `internal/icons`). La paleta tiene 6 colores, todos ≥ 4.5:1 sobre blanco: verde `#0F6E61`, azul `#1D4ED8`, ámbar `#9A4A06`, rojo `#B42318`, violeta `#6D28D9` y gris `#4D525A`. Van en la carta del jugador; el nombre y la descripción salen en el tooltip.
  - **Puntajes**: el color del número (acento ≥ 85, tinta 70–84, apagado < 70) nunca va solo, siempre con el número escrito.

- **El contenido es ilustrativo.** Stitch completó logros, stacks y skills por su cuenta (p. ej. le puso Rust/Tokio/gRPC a Gremio, que es Go, y agregó GCP, PCI-DSS y BGP). El contenido real sale del perfil importado (`duv0-x.github.io/files/cv.yaml`). Tampoco vale el email que arma el botón "Mostrar email" del HTML.
- **Privacidad**: ninguna pantalla muestra edad, género, nacionalidad, foto obligatoria ni salarios (revisado en capturas y HTML). El contacto aparece oculto por defecto.
- **Layout responsivo**: en móvil todo va en una columna, con gutter de 16 px, botones a ancho completo y tabs con scroll horizontal. En el panel, la navegación lateral pasa a una barra inferior de 4 ítems y las acciones del formulario quedan al pie. Las pantallas que no tienen versión móvil propia (vista por rol, vistas, importar y sesiones) siguen ese mismo patrón. `/cv` es solo de impresión.
- **`/cv`**: Stitch usó mono para todo el cuerpo. En el código el cuerpo va en IBM Plex Sans, y en mono solo las fechas y los datos.
- **Elementos que no se implementan tal cual**: el contador "Storage 12.4 MB · 0 rastreadores" del panel, los "Skills auditados" y las cifras de federación son adornos de Stitch y no están en [ADR-003](../adr/ADR-003-perfil-v1.md).
