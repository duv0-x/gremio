---
title: "Autenticación del panel: enlace de un solo uso por email"
number: 9
date: 2026-10-24
domain: security
status: "aceptado"
tags: [auth, magic-link, email, smtp, session, csrf, security]
---

# ADR-009: Autenticación del panel: enlace de un solo uso por email

## Contexto

El panel de configuración ([ADR-008](ADR-008-frontend.md)) lo usa una sola persona: el dueño de la instancia ([ADR-001](ADR-001-instancia-y-federacion.md)). No hay registro ni otras cuentas. Hay que decidir cómo se autentica, cuánto dura la sesión y cómo se protege el panel, sin guardar contraseñas.

## Decisión

### 1. Enlace de un solo uso enviado por email

- **El email del dueño** se configura por CLI: `gremio admin init --email` y `gremio admin set-email`. Se guarda en la base.
- **La página `/login` no pide ningún dato**: solo tiene un botón "Enviarme un enlace", porque hay un único destinatario posible. No hay forma de enumerar cuentas ni de mandar correos a terceros.
- **El enlace** lleva un token de 32 bytes aleatorios (`crypto/rand`, base64url):
  - La base guarda solo su **hash SHA-256**.
  - Caduca a los **15 minutos**, es de **un solo uso** y pedir uno nuevo invalida el anterior.
- **Límite de envíos**: 1 enlace por minuto y 5 por hora, para que nadie llene el buzón del dueño desde la página de login.
- El enlace apunta al `host` de la instancia, que siempre es `https`.

### 2. Abrir el enlace no inicia sesión: hay que pulsar un botón

- Los escáneres de seguridad de correo (Outlook, Gmail corporativo, antivirus) abren los enlaces antes que la persona. Si el `GET` gastara el token, el enlace llegaría inservible.
- Por eso `GET /login/<token>` solo muestra una página con el botón **"Entrar al panel"**. El token se valida y se gasta en el `POST`.
- El enlace **no queda atado al navegador** que lo pidió: se puede pedir en la laptop y abrir en el celular.
- Esa página responde con `Referrer-Policy: no-referrer` y no carga recursos externos ([ADR-008](ADR-008-frontend.md)), para que el token no se filtre.

### 3. Envío por SMTP genérico

- `net/smtp` de la biblioteca estándar, sin dependencias. Funciona con cualquier proveedor que exponga SMTP.
- Configuración por flags o variables: `GREMIO_SMTP_HOST`, `GREMIO_SMTP_PORT`, `GREMIO_SMTP_USERNAME`, `GREMIO_SMTP_PASSWORD` y `GREMIO_SMTP_FROM`. La clave solo viene del entorno, nunca de un flag, que quedaría en el historial del shell y en `ps`.
- **Siempre con TLS**: STARTTLS en el puerto 587 o TLS implícito en el 465. Gremio se niega a autenticarse sin TLS.
- `serve` avisa en el log al arrancar si el SMTP no está configurado.

### 4. Sin acceso de respaldo

- **No hay otra forma de entrar al panel**: ni contraseña ni enlace por CLI. Si el correo no llega, hay que arreglar la configuración SMTP.
- `gremio admin set-email` cambia el destinatario, pero no inicia sesión.

### 5. Sesión de 30 días, renovable

- Al entrar se crea una sesión con un token aleatorio de 32 bytes. La base guarda su hash y su vencimiento, y la sesión se puede revocar.
- **Cookie `__Host-gremio_session`**: `HttpOnly`, `Secure`, `SameSite=Lax`, `Path=/` y sin `Domain`. El prefijo `__Host-` obliga al navegador a respetar esas tres últimas condiciones.
- Dura **30 días y se renueva al usarla**. Para no escribir en cada petición, la fecha de vencimiento se actualiza como mucho una vez al día.
- Cerrar sesión borra la sesión de la base. Desde el panel se pueden cerrar todas.

### 6. Protección contra CSRF

- Las rutas del panel que cambian estado van detrás de **`http.CrossOriginProtection`** (Go 1.25+). Rechaza peticiones de otro origen usando `Sec-Fetch-Site` y `Origin`, sin tokens CSRF a mano.
- Se suma `SameSite=Lax` en la cookie.

## Consecuencias positivas

- No se guarda ninguna contraseña: no hay nada que filtrar, adivinar ni reutilizar.
- Una página de login sin campos elimina la enumeración de cuentas y el envío de correos a terceros.
- La página con botón vuelve el enlace inmune a los escáneres de correo.
- Todo es biblioteca estándar: `net/smtp`, `crypto/rand` y `http.CrossOriginProtection`.

## Consecuencias negativas

- **Entrar depende del correo.** Si el SMTP falla o el proveedor bloquea los envíos, el dueño queda fuera del panel hasta arreglarlo. Fue una decisión explícita (sin respaldo).
- La seguridad del panel es la del buzón del dueño: quien controle ese correo controla la instancia.
- Cada instancia necesita una cuenta SMTP. Para alguien no técnico es una barrera de instalación.
- `net/smtp` está congelado y no recibirá funciones nuevas (p. ej. OAuth2 para Gmail): sirve para proveedores con usuario y clave o *app passwords*.

## Alternativas consideradas

| Alternativa | Razón de descarte |
|-------------|-------------------|
| Contraseña | Hay que guardarla bien (hash lento), gestionar recuperación y defenderse de fuerza bruta |
| Passkeys (WebAuthn) | Más seguras, pero la recuperación al perder el dispositivo vuelve a necesitar otro mecanismo; candidata para más adelante |
| Solo acceso local (`127.0.0.1`) | Obliga a un túnel SSH para editar el perfil |
| API de un proveedor de correo | Ata Gremio a un proveedor concreto |
| Enlace de respaldo por CLI | Descartado por el autor: un solo camino de acceso |
| Enlace atado al navegador que lo pidió | Más resistente a phishing, pero impide pedirlo en un dispositivo y abrirlo en otro |
| Sesión de 7 días o de navegador | Más fricción sin ganancia clara para una instancia de una persona |

## Links relacionados

- [ADR-001](ADR-001-instancia-y-federacion.md): dueño único y `host` con `https`.
- [ADR-008](ADR-008-frontend.md): panel renderizado en el servidor y sin recursos externos.
