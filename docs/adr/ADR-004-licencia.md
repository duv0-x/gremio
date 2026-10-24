---
title: "Licencias del software, de la especificación y de las contribuciones"
number: 4
date: 2026-10-24
domain: governance
status: "aceptado"
tags: [license, agpl, cc0, dco, governance]
---

# ADR-004: Licencias del software, de la especificación y de las contribuciones

## Contexto

La meta de fondo es cambiar cómo se recluta con una red abierta. El riesgo principal es que una plataforma de empleo tome el código, lo modifique y lo ofrezca como servicio cerrado, rompiendo la red. Al mismo tiempo, la red crece si otros pueden implementar el mismo vocabulario de perfiles y proyectos, aunque sea en software propio.

Ya hay piezas con licencia definida: el núcleo copiado de go-ap en `internal/goap/` es MIT ([ADR-001](ADR-001-instancia-y-federacion.md)), y el contenido de los proyectos es CC BY 4.0 por defecto ([ADR-002](ADR-002-proyecto-y-ejecucion.md)).

## Decisión

### 1. Software: AGPL-3.0-or-later

- Todo el código del proyecto se licencia bajo **GNU AGPL-3.0-or-later** (`LICENSE` en la raíz, texto oficial de gnu.org).
- "Or later" permite adoptar una futura versión de la AGPL publicada por la FSF sin pedir permiso a cada contribuidor.
- `internal/goap/` **mantiene su licencia MIT** y sus archivos `LICENSE` originales. MIT es compatible con AGPL: el binario combinado se distribuye bajo AGPL, y el código copiado conserva su aviso.

### 2. Enlace al código fuente desde cada instancia

La sección 13 de la AGPL obliga a ofrecer el código a quienes usan una versión modificada por red. Cada instancia muestra un enlace "Source" en el pie de página, y el repo al que apunta se puede configurar. Así quien corre un fork cumple sin esfuerzo, como en el fork de GoToSocial `echo.duv0-x.cloud`.

### 3. Especificación y contexto JSON-LD: CC0

- La especificación del vocabulario (perfil, vistas, proyecto, ejecución) y el documento del contexto JSON-LD se publican bajo **CC0 1.0** (dominio público), en `docs/spec/` cuando existan.
- Cualquiera, incluido software cerrado, puede implementar el vocabulario e interoperar. La AGPL protege *esta* implementación; CC0 hace que *la red* pueda crecer con otras.

### 4. Contribuciones: DCO

- Cada commit lleva `Signed-off-by: Nombre <email>` (`git commit -s`), certificando el [Developer Certificate of Origin 1.1](https://developercertificate.org/).
- No hay CLA: cada contribuidor conserva su copyright y aporta bajo la licencia del repo.
- Detalle en `CONTRIBUTING.md`.

## Consecuencias positivas

- Nadie puede ofrecer una versión modificada como servicio sin publicar su código: la red no se puede cerrar.
- Es el modelo de licencia de Mastodon y GoToSocial, conocido en el fediverso.
- CC0 en la especificación evita que la licencia del software frene implementaciones alternativas compatibles.
- DCO no tiene fricción para contribuir.

## Consecuencias negativas

- Algunas empresas prohíben usar o contribuir a software AGPL: menos adopción corporativa.
- Sin CLA, relicenciar el proyecto (p. ej. vender licencias comerciales) requerirá el acuerdo de cada contribuidor.
- Hay que mantener la separación de licencias dentro del repo (AGPL general, MIT en `internal/goap/`, CC0 en `docs/spec/`).

## Alternativas consideradas

| Alternativa | Razón de descarte |
|-------------|-------------------|
| MIT | Permite el SaaS cerrado que la AGPL busca impedir |
| Apache-2.0 | Igual que MIT frente a SaaS cerrados (su ventaja son las patentes) |
| MPL-2.0 | Copyleft por archivo; no cubre el uso como servicio |
| GPL-3.0 (la de Forgejo) | No cubre el uso por red: un SaaS modificado no tiene que publicar nada |
| AGPL-3.0-only | Cambiar de versión requeriría a todos los contribuidores |
| CLA | Ahuyenta contribuidores; solo vale si se planea licenciamiento dual |
| Especificación bajo AGPL | Puede frenar implementaciones alternativas de la red |

## Links relacionados

- Texto de la licencia: [`LICENSE`](../../LICENSE) (fuente: https://www.gnu.org/licenses/agpl-3.0.txt).
- Código MIT de terceros: [`internal/goap/README.md`](../../internal/goap/README.md).
- [`CONTRIBUTING.md`](../../CONTRIBUTING.md).
