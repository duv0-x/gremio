---
title: "Lenguaje y distribución: Go, binario único e imagen OCI"
number: 7
date: 2026-10-24
domain: architecture
status: "aceptado"
tags: [go, distribution, single-binary, oci, goreleaser, release]
---

# ADR-007: Lenguaje y distribución: Go, binario único e imagen OCI

## Contexto

Desde el principio la idea fue "en el espíritu de GoToSocial: un binario, fácil de desplegar", y los ADRs 001 a 006 asumen Go, pero ninguno lo decide. Cada persona corre su propia instancia ([ADR-001](ADR-001-instancia-y-federacion.md)), así que instalar y actualizar tiene que ser trivial: desde una Raspberry Pi con 1 GB hasta un clúster de Kubernetes. La prioridad del proyecto es una plataforma descentralizada y robusta.

## Decisión

### 1. Lenguaje: Go

- Todo el servidor se escribe en **Go**, en la última versión estable (hoy 1.26; el `go.mod` declara la mínima).
- Por qué Go: binarios estáticos sin runtime, compilación cruzada trivial (linux/arm64 para la Pi), concurrencia nativa para el inbox y la cola de entrega, y una biblioteca estándar fuerte en HTTP y criptografía. Es además el lenguaje de GoToSocial, el referente directo del proyecto.
- **Go puro, `CGO_ENABLED=0`**: ninguna dependencia puede requerir cgo. Lo hace posible el driver de SQLite de ncruces ([ADR-006](ADR-006-almacenamiento.md)).

### 2. Un binario con todo adentro

- Un solo ejecutable `gremio` con subcomandos (`serve`, `admin`, `backup`…).
- Plantillas, CSS, JS (htmx) y migraciones se embeben con `embed` ([ADR-008](ADR-008-frontend.md)). En producción no se lee nada del disco salvo la configuración y el directorio de datos.
- **El código generado se commitea** (sqlc, templ y el CSS compilado de Tailwind): `go build` funciona sin ninguna otra herramienta. CI regenera y falla si hay diferencias.

### 3. Distribución: binarios + imagen OCI

- Cada release publica:
  - **binarios** para linux/amd64, linux/arm64 y darwin/arm64, con checksums;
  - una **imagen OCI multi-arch** (amd64 + arm64) en GitHub Container Registry, sobre una base mínima sin shell (distroless `static`, usuario no root), con el directorio de datos como volumen.
- Las releases se generan con **GoReleaser** (MIT) desde GitHub Actions al crear un tag `vX.Y.Z`. Es el mismo enfoque que sigue `echo.duv0-x.cloud`.
- **El Helm chart se agrega cuando haya algo que desplegar en Kubernetes**, no antes.
- Versionado semántico. Mientras sea `0.x`, puede haber cambios incompatibles entre versiones menores.

## Consecuencias positivas

- Instalar es bajar un archivo o correr una imagen; actualizar es reemplazarlo.
- La misma build sirve para la Pi, un VPS, la Mac y Kubernetes.
- `go build` sin herramientas extra: cualquiera que clone el repo compila.
- Con distroless y sin cgo, la superficie de ataque de la imagen es mínima.

## Consecuencias negativas

- La prohibición de cgo descarta librerías que lo requieren (p. ej. algunos drivers o librerías de imagen).
- Commitear código generado agrega ruido a los diffs y exige que CI verifique que está al día.
- Sin Helm chart al inicio, desplegar en Kubernetes requiere manifiestos propios.

## Alternativas consideradas

| Alternativa | Razón de descarte |
|-------------|-------------------|
| Rust | Igual de apto para binarios únicos, pero más lento de escribir; Go ya fue elegido por el autor |
| Node / TypeScript (p. ej. Fedify) | Necesita runtime instalado; más difícil de empaquetar en un binario |
| Solo binario, sin imagen | La imagen es lo que se usa en Kubernetes y en cualquier host con Docker |
| Helm chart desde el primer release | Mantener un chart antes de que haya algo que desplegar |

## Links relacionados

- [ADR-006](ADR-006-almacenamiento.md): ncruces hace posible `CGO_ENABLED=0`.
- [ADR-008](ADR-008-frontend.md): los assets que se embeben.
- Referencia de releases: `/Users/lx-duv0-x/Documents/Repositories/echo.duv0-x.cloud` (`release.yml`).
