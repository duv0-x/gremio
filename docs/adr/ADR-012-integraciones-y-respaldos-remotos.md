---
title: "Integraciones y respaldos remotos: proveedores incluidos, cifrado con age y Google Drive"
number: 12
date: 2026-11-05
domain: architecture
status: "propuesto"
tags: [plugins, integrations, backup, encryption, age, google-drive, oauth]
---

# ADR-012: Integraciones y respaldos remotos

## Contexto

`gremio backup` (gremio#101) escribe un `.tar.gz` con la base, `cv.pdf` y `media/`, pero lo deja en el mismo disco que la instancia. En una Raspberry Pi con una tarjeta SD, si falla el disco se pierden la instancia y sus respaldos a la vez. El dueño quiere que cada instancia pueda **conectar un proveedor de almacenamiento** (Google Drive, MEGA, etc.) y mandar ahí sus respaldos: es el primer caso de lo que llamamos "plugins" de la plataforma.

El respaldo trae la **llave privada** de la instancia, que firma todo lo que publica por ActivityPub ([ADR-001](ADR-001-instancia-y-federacion.md)), y los tokens de las integraciones conectadas. No puede salir de la máquina en claro.

Restricciones: Go puro, sin cgo, y un binario con todo adentro ([ADR-007](ADR-007-lenguaje-y-distribucion.md)). Configuración desde el panel del dueño ([ADR-009](ADR-009-autenticacion-del-panel.md)).

## Decisión

### 1. Un "plugin" es una integración incluida en el binario

- Cada proveedor es un **paquete Go dentro de Gremio** (`internal/integrations/<proveedor>`) que implementa una interfaz de su categoría. El dueño los conecta, configura y desconecta desde el panel. No se cargan plugins de terceros.
- La primera categoría es **destino de respaldos**:

  ```go
  type Destination interface {
      Upload(ctx context.Context, name string, r io.Reader) error
      List(ctx context.Context) ([]string, error) // nombres de los respaldos de esta instancia
      Delete(ctx context.Context, name string) error
  }
  ```

- La primera integración es **Google Drive**. Le siguen otras sobre la misma interfaz (MEGA, S3 compatible, WebDAV) cuando haga falta, cada una en su PR.
- Las credenciales y los tokens se guardan en la base, como la llave privada: la base es `0600` y su respaldo va cifrado (§2).

### 2. Todo respaldo que sale de la máquina va cifrado con age

- Cifrado con [age](https://age-encryption.org) (`filippo.io/age`, Go puro, BSD-3) para un destinatario X25519.
- **La instancia guarda solo la clave pública.** El panel genera el par una vez y muestra la clave secreta (`AGE-SECRET-KEY-1…`) **una sola vez**, para que el dueño la guarde en su gestor de contraseñas. Gremio no puede descifrar sus propios respaldos remotos: ni quien robe el disco ni quien entre a la cuenta del proveedor puede leerlos.
- El archivo remoto es `gremio-<fecha>.tar.gz.age`. Restaurar: `age -d -i clave.txt archivo.age | tar -xz -C <data-dir vacío>` (o `rage`), documentado en el README.
- Si el dueño pierde la clave secreta, los respaldos remotos no sirven. Puede generar otra; los respaldos siguientes usan la nueva.
- Sin clave configurada no hay subida: el panel no deja conectar un destino hasta generarla.

### 3. Respaldos programados dentro de `serve`

- `serve` corre el respaldo según la **frecuencia** elegida en el panel (por defecto, cada 24 horas): `gremio backup` (gremio#101) → cifrar → subir a cada destino conectado → borrar los remotos que pasan la **retención** (por defecto, los 7 más recientes). La copia local temporal se borra al terminar.
- El panel muestra el último resultado (hora, tamaño, error) y un botón **"Respaldar ahora"**. Un fallo se registra en el log y en el panel; no detiene la instancia.
- Solo se borran archivos que la propia instancia subió y que siguen el patrón `gremio-<fecha>.tar.gz.age`.

### 4. Google Drive con el cliente OAuth del dueño y el permiso `drive.file`

- **Cada instancia usa su propio cliente OAuth** de Google Cloud, como usa su propio SMTP ([ADR-009](ADR-009-autenticacion-del-panel.md)): `GREMIO_GOOGLE_CLIENT_ID` y `GREMIO_GOOGLE_CLIENT_SECRET` (el secreto solo por entorno). No hay una app central del proyecto: la plataforma es descentralizada y Gremio no intermedia credenciales de nadie. El redirect es `https://<host>/panel/integraciones/google/callback`.
- **Permiso `https://www.googleapis.com/auth/drive.file`**: Gremio solo ve los archivos que él crea, no el resto del Drive. Google lo clasifica como **no sensible**.
- El panel hace el flujo OAuth (con `state` y PKCE), guarda el **refresh token** y crea una carpeta "Gremio – <handle>" donde sube los respaldos.
- **La app de Google tiene que estar en "In production"**: en "Testing", Google emite refresh tokens que vencen a los 7 días. Con solo permisos no sensibles, pasar a producción no debería exigir verificación de Google. Riesgo abierto: hay un reporte (sin confirmar) de expiración a los 7 días también en producción. Se valida en la Pi durante más de 7 días antes de dar el plugin por terminado.
- Sin la librería `google.golang.org/api` (enorme): `golang.org/x/oauth2` más las cuatro llamadas REST de Drive v3 que hacen falta (crear carpeta, subir con `multipart`, listar con `q`, borrar).

## Consecuencias positivas

- Los respaldos sobreviven a la pérdida del disco de la instancia, sin que el dueño haga nada después de conectarlo.
- El proveedor nunca ve el contenido, y Gremio tampoco puede descifrarlo: perder el control de la cuenta de Drive o del disco no expone la llave privada.
- Sigue siendo un binario sin cgo. Agregar un proveedor es implementar tres métodos.
- `drive.file` limita el daño si el token se filtra: solo alcanza los archivos que creó Gremio.

## Consecuencias negativas

- Solo los mantenedores agregan proveedores; un tercero no puede publicar su propio plugin.
- El dueño tiene que crear un cliente OAuth en Google Cloud y pasarlo a producción. Es un paso técnico que la documentación tiene que guiar bien.
- Si pierde la clave secreta de age, sus respaldos remotos son irrecuperables.
- Respaldos completos cada vez, sin incrementales. Con una instancia de pocos MB es aceptable.

## Alternativas consideradas

| Alternativa | Por qué no |
|-------------|-----------|
| Plugins externos en WebAssembly (wazero) | Abre la plataforma a terceros, pero exige una API de plugins, permisos y red desde WASM: mucho diseño para un primer proveedor |
| Plugins como procesos aparte (hashicorp/go-plugin) | Rompe "un binario con todo adentro" (ADR-007) |
| rclone como librería | Cubre 70+ proveedores, pero suma decenas de MB y dependencias al binario |
| Una app OAuth central del proyecto | Gremio pasaría a intermediar credenciales de todas las instancias, en contra de la descentralización; además, su verificación y cuota serían un punto único de falla |
| Cifrado opcional | El respaldo lleva la llave privada: subirlo en claro a un tercero no es aceptable |
| Cifrar con una clave que guarda la instancia | Quien robe el disco tendría también la clave de los respaldos remotos |

## Links relacionados

- gremio#101 (respaldo completo) y gremio#107 (este plugin).
- Ámbitos de la API de Drive: https://developers.google.com/workspace/drive/api/guides/api-specific-auth
- Expiración de refresh tokens: https://developers.google.com/identity/protocols/oauth2#expiration
- age: https://age-encryption.org
