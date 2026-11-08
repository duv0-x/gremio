# Catálogo revisado de retos

Retos del dojo que viajan en el binario de Gremio (ADR-013 §8). Un archivo por versión: `<slug>.v<N>.json`, con el formato de `retos.Challenge`.

## Cómo proponer un reto

1. Abre un PR que agregue `<slug>.v1.json`. Una versión nueva es un archivo nuevo (`.v2.json`): las versiones publicadas no se editan.
2. Declara `author` (tu handle de GitHub) y `origin` (`human` o `ai`; si es `ai`, también `model`). La IA nunca es autora ni revisora: quien adopta el borrador queda como `author`.
3. Cada habilidad usa un id de `internal/dojo/skills.go`, con 3 como máximo y ningún nivel por encima del nivel del reto.
4. Trae al menos 4 checks, diseñados para que solo pasen si el reto se resolvió de verdad: varias piezas de estructura y de contenido, no un solo archivo trivial.
5. **Revisión:** alguien distinto del autor revisa el PR y su handle se agrega en `reviewer`. Sin `reviewer`, o con `reviewer` igual a `author`, el reto queda **en revisión** y cuenta como práctica propia (tope N2) aunque esté en el repo.

## Qué prueban los checks y qué no

Los checks son **declarativos**: Gremio baja el tarball del repo en el commit fijado (github.com, codeberg.org o gitlab.com) y comprueba que existan archivos (`file`), que contengan una expresión regular (`contains`) o que no existan (`absent`). **Nunca ejecuta el código de la entrega.**

Por eso son evidencia de estructura y contenido, no de que el código funcione: un repo puede tener la forma correcta sin pasar sus propios tests. Ese límite es la razón del tope N3. Correr los tests (el resultado del CI de un workflow fijado por el catálogo, firmado con Sigstore) es el camino a N4 y será otro `kind` de check.

## Límites de lectura

Tarball de 50 MiB comprimido y 200 MiB descomprimido como máximo, 50 000 archivos, 1 MiB por archivo revisado con `pattern` y 60 s de timeout. Solo se leen archivos regulares: los symlinks, hardlinks y rutas con `..` se ignoran.
