---
title: "Plugins: integraciones incluidas que el dueño habilita y configura desde el panel"
number: 12
date: 2026-11-05
domain: architecture
status: "aceptado"
tags: [plugins, integrations, panel, secrets, ollama, claude]
---

# ADR-012: Plugins

## Contexto

El dueño de una instancia quiere conectar servicios externos sin tocar archivos ni la línea de comandos: una sección **Plugins** en el panel ([ADR-009](ADR-009-autenticacion-del-panel.md)) donde ve los plugins disponibles, los habilita dándoles lo que piden (por ejemplo, un token) y los deshabilita cuando quiera. Los primeros candidatos:

- **Ollama** (Ollama Cloud o una instalación propia) y **Claude**: modelos de IA con un token o una API key.
- **Google Drive:** respaldos fuera de la máquina.
- **Más adelante:** el módulo de proyectos que el usuario resuelve para sumar puntos en sus skills.

Qué puede hacer cada plugin se decide aparte. Este ADR decide **qué es un plugin, cómo se habilita, dónde se guardan sus credenciales y cómo se ve en el panel**.

Restricciones: Go puro, sin cgo y un binario con todo adentro ([ADR-007](ADR-007-lenguaje-y-distribucion.md)). Panel renderizado en el servidor que funciona sin JavaScript ([ADR-008](ADR-008-frontend.md)).

## Decisión

### 1. Un plugin es una integración incluida en el binario

- Cada plugin es un paquete Go (`internal/plugins/<id>`) que se registra en un **catálogo** al compilar. No se cargan plugins de terceros en tiempo de ejecución.
- Cada plugin declara:
  - un **id** estable (`ollama-cloud`), con su **nombre**, una **descripción** de una línea y una **categoría** (IA, Respaldos…);
  - los **campos** que pide para habilitarse. Cada campo tiene etiqueta, ayuda y tipo: `text` o `secret`. Cuando un plugin lo necesite se agregarán tipos como "conectar con OAuth";
  - un **chequeo** que valida esos datos contra el servicio y devuelve un resumen para el panel ("Conectado como Ana · plan Pro") o un error legible.

  ```go
  type Plugin interface {
      Info() Info                                            // id, nombre, descripción, categoría, campos
      Check(ctx context.Context, cfg Config) (string, error) // valida la configuración; el string va al panel
  }
  ```

- **Lo que hace cada plugin una vez habilitado** (sus capacidades) se agrega por categoría, con su propio issue o ADR. El sistema de plugins solo gestiona el catálogo, la configuración y el estado.

### 2. Configuración en la base; los secretos no vuelven a salir

- Tabla `plugins`: `id`, `config` (JSON), `status` (el resumen o el error del último chequeo), `checked_at` y `enabled_at`. **Un plugin está habilitado si tiene fila**: no hay estado intermedio.
- Los secretos se guardan en la base como la llave privada de la instancia: el archivo es `0600` y entra en `gremio backup`. **El panel nunca los devuelve**: muestra `•••• abcd` (los últimos 4) y permite reemplazarlos.
- **Deshabilitar borra la fila**, con sus credenciales. Para volver a habilitarlo, el dueño vuelve a dar los datos. No quedan tokens viejos olvidados en la base.
- Las llamadas a los servicios las hace el servidor, con timeout de 10 s. Los tokens nunca llegan al navegador.

### 3. Sección Plugins en el panel

- `/panel/plugins` muestra una tarjeta por plugin del catálogo: nombre, categoría, descripción y estado (habilitado con su resumen, o "Deshabilitado").
- **Habilitar** abre el formulario con sus campos. Al guardar, Gremio corre el chequeo:
  - si pasa, guarda la configuración y la tarjeta queda **Habilitado** con el resumen;
  - si falla, no guarda nada y el formulario vuelve con el error.
- En un plugin habilitado:
  - **Probar conexión** repite el chequeo;
  - **Editar** reemplaza los datos (los secretos vacíos conservan el valor actual);
  - **Deshabilitar** pide confirmación y borra la configuración.
- Formularios normales con POST, como el resto del panel, más htmx cuando esté disponible. Mismos componentes de [DESIGN.md](../design/DESIGN.md).

### 4. Primeros plugins: Ollama y Claude

**Ollama** (`ollama`, categoría IA) sirve para Ollama Cloud y para instalaciones propias.

- **Pide:**
  - **Endpoint** (`text`): por defecto `https://ollama.com`, que es Ollama Cloud. Una instalación propia usa su URL, p. ej. `http://192.168.1.50:11434`. Solo `http` o `https`.
  - **Token** (`secret`): obligatorio con Ollama Cloud y opcional en una instalación propia, que normalmente no lo usa.
- **Chequeo:**
  1. `GET <endpoint>/api/tags`, con el token si hay. Tiene que responder la lista de modelos; si no, "No se pudo conectar con Ollama en <endpoint>".
  2. Con token: `POST <endpoint>/api/me`. Con 200 el resumen es "Conectado como <Name> · plan <Plan>"; con 401 o 403, "Ollama rechazó el token". Un 404 se ignora, porque una instalación propia puede no tener ese endpoint.
  3. Sin `/api/me`, el resumen es "<N> modelos en <host>".
- **Por qué dos llamadas:** en Ollama Cloud `GET /api/tags` responde 200 incluso sin token (probado el 2026-11-05), así que no sirve para validarlo. Por eso, con el endpoint de Ollama Cloud el token es obligatorio. `/api/me` es el endpoint que usa el propio Ollama, pero no está en su documentación pública.

**Claude** (`claude`, categoría IA) usa la API de Anthropic.

- **Pide:** **API key** (`secret`), creada en la consola de Anthropic.
- **Chequeo:** lista los modelos con el SDK oficial de Go (`github.com/anthropics/anthropic-sdk-go/v2`, `client.Models.List`).
  - Si responde, el resumen es "<N> modelos disponibles".
  - Un `*anthropic.Error` con 401 da "Anthropic rechazó la API key"; con otro código, el mensaje de error de la API.
- **SDK y no HTTP a mano:** el plugin va a usar la API de mensajes cuando se decida qué hace, y el SDK es el cliente soportado. Es Go puro.

**Qué hacen** los dos una vez habilitados se decide después, con el módulo de proyectos. Mientras tanto validan y guardan su configuración, y dejan un cliente listo para usar.

### 5. Respaldos remotos y Google Drive, en un ADR propio

Google Drive necesita OAuth con el cliente propio de cada instancia, respaldos programados y cifrado. La propuesta previa: age, con la instancia guardando solo la clave pública y el dueño la secreta. Se decide cuando se implemente, sobre este mismo sistema de plugins.

## Consecuencias positivas

- El dueño habilita integraciones desde el panel, sin archivos de configuración ni reinicios.
- Agregar un plugin es implementar `Info` y `Check` y registrarlo. El panel, el almacenamiento y el estado ya existen.
- Los tokens no salen del servidor y no se pueden leer desde el panel. Deshabilitar no deja credenciales guardadas.
- Sigue siendo un binario sin cgo.

## Consecuencias negativas

- Solo los mantenedores agregan plugins; un tercero no puede publicar el suyo.
- Los secretos están en la base en claro, protegidos por los permisos del archivo, igual que la llave privada. Quien lea `gremio.db` o un respaldo sin cifrar los ve.
- El chequeo del token de Ollama Cloud depende de un endpoint que no está documentado.
- El endpoint de Ollama lo elige el dueño: el servidor hace peticiones a la URL que él ponga, incluida su red local. Es lo que permite usar una instalación propia, y el panel solo lo usa el dueño.
- Volver a habilitar un plugin exige dar otra vez sus datos.

## Alternativas consideradas

| Alternativa | Por qué no |
|-------------|-----------|
| Plugins externos en WebAssembly (wazero) | Abre la plataforma a terceros, pero exige una API de plugins, permisos y red desde WASM. Se puede sumar después sin cambiar el panel |
| Plugins como procesos aparte (hashicorp/go-plugin) | Rompe "un binario con todo adentro" (ADR-007) |
| Configurar los plugins por variables de entorno, como el SMTP | Obliga al dueño a editar archivos y reiniciar; el pedido es hacerlo desde el panel |
| Cifrar los secretos con una clave de la instancia | La clave viviría junto a la base: no protege de quien tenga el disco, y complica restaurar |
| Deshabilitar sin borrar la configuración | Quedarían tokens guardados de servicios que el dueño ya no usa |

## Links relacionados

- gremio#107: sistema de plugins y primer plugin.
- gremio#101: respaldo completo, la base de los respaldos remotos.
