# Contexto del proyecto

## Propósito y alcance

`commit_tool` es el proyecto Go para migrar la experiencia de `gitmoji-cli`
a un ejecutable nativo. El módulo es `github.com/Snah-s/commit_tool`.
El producto debe funcionar offline desde el primer uso, sin Node, npm, Yarn
ni npx, y utilizar el Git instalado.

La primera versión incluye tres modos de mensaje, selección y búsqueda inline,
configuración JSON, catálogo de emojis, cliente de commit y hook
`prepare-commit-msg`. Los contratos detallados se mantienen en [PLAN](PLAN.md).
El estado de implementación se mantiene en [MEMORY](MEMORY.md).
El [contrato de compatibilidad](COMPATIBILITY.md) recoge las decisiones de fase 0
y las diferencias verificadas respecto del upstream fijado.

## Lenguaje compartido

| Término | Significado |
| --- | --- |
| Modo | Formato del mensaje: `emoji`, `standard` o `hybrid`. |
| Tipo | Categoría de cambio: `feat`, `fix`, `docs`, `refactor`, `test` o `chore`. |
| Alcance / scope | Módulo afectado, opcional; no es una categoría adicional. |
| Descripción | Texto breve del cambio, sin prefijos; `--title` proporciona este campo. |
| Título | Cabecera completa, con prefijos y separadores; sobre ella se cuenta la longitud. |
| Cuerpo | Contenido opcional tras una línea vacía, incluidos párrafos y trailers. |
| Catálogo | Entradas de gitmoji identificadas por `code`, con emoji, nombre y descripción. |
| Clasificación | Tabla local que asocia códigos de emoji a los seis tipos; puede haber asociaciones múltiples. |
| Cliente | Flujo que recopila respuestas y ejecuta `git commit`. |
| Hook propio | Hook reconocido por marcador y contenido de la herramienta; su mera existencia no acredita propiedad. |
| Configuración de proyecto | Primera configuración válida encontrada desde cwd hacia los padres. |
| Configuración global | Preferencias del usuario, usadas cuando no hay configuración de proyecto. |
| Cancelación | Decisión del usuario de interrumpir el flujo; distinta de un fallo de lectura, escritura o Git. |
| Sin TTY | Ausencia de terminal interactivo; no permite abrir prompts invisibles. |

## Decisiones e invariantes

- Los tres modos son parte del alcance confirmado. `emoji` conserva el prefijo
  gitmoji; `standard` empieza por tipo y no añade emoji; `hybrid` usa tipo y
  emoji clasificado. Los formatos exactos y reglas están en PLAN 3.
- El default es `standard`; el híbrido coloca el emoji después de `: `.
  El ejecutable se llama `gitmoji`. Linux/macOS amd64 y arm64 son el objetivo
  inicial; Windows amd64 depende de validar sus hooks antes de anunciar soporte.
- El perfil inicial usa exclusivamente los seis tipos del glosario. La recomendación
  de 72 caracteres es un aviso sobre puntos de código Unicode del título completo,
  no un máximo obligatorio.
- Cuerpo, trailers y datos preexistentes se conservan salvo descarte explícito.
  La validación semántica del imperativo y del tipo requiere revisión humana.
- En híbrido, un emoji incompatible se limpia al cambiar tipo y se vuelve a elegir.
  Los emojis nuevos sin clasificación siguen disponibles en modo emoji.
- La UI mantiene el scrollback, ofrece accesibilidad y restaura el terminal;
  la finalización del commit requiere una acción explícita.
- El uso normal no exige red; la actualización del catálogo es explícita.
  Una actualización fallida conserva la caché utilizable.
- Git sigue siendo una dependencia externa. Los hooks ajenos, staging e identidad
  del usuario se respetan; `autoAdd` es false por defecto.
- Las diferencias de compatibilidad se documentan; no se retiran herramientas
  heredadas antes de aceptar los contratos y la distribución nativa.

## Dirección técnica

Go es el lenguaje del proyecto; la versión mínima está en `go.mod`. Huh es la
primera opción para el prototipo de formularios. Bubble Tea/Bubbles se incorporan
solo si ese prototipo demuestra una necesidad; no se mantienen dos flujos de UI.
Las versiones y APIs de Charm se verifican antes de añadir dependencias.

Se priorizan `flag`, `encoding/json`, `net/http`, `os/exec`, `embed` y `testing`.
La construcción del mensaje es una operación sin efectos externos. La organización
prevista es:

| Ruta objetivo | Responsabilidad |
| --- | --- |
| `cmd/gitmoji/main.go` | Entrada, ayuda, versión y salida. |
| `internal/cli/` | Argumentos y despacho de comandos. |
| `internal/commit/` | Parseo, validación y formato compartidos. |
| `internal/ui/` | Formularios, búsqueda y estado visual. |
| `internal/config/` | Preferencias, precedencia e importación. |
| `internal/catalog/` | Catálogo, clasificación, caché y actualización; assets dentro del paquete. |
| `internal/git/` | Procesos Git y ciclo de vida del hook. |

Estas rutas describen responsabilidades futuras, no paquetes ya implementados.
Servicios, bases de datos, plugins y pantalla alternativa no forman parte del alcance.

## Cómo consultar el contrato

| Trabajo | Secciones de PLAN |
| --- | --- |
| Formatos, clasificación y validación | 3 |
| Stack, UI y terminal | 4–5 |
| Comandos y aliases | 6 |
| Configuración, caché y red | 7 |
| Cliente y hooks | 8 |
| Recursos heredados e importación por archivo | 9–9.1 |
| Fases, modelos recomendados y criterios de salida | 10–10.1 |
| Casos de verificación y riesgos | 11–12 |

Las preferencias nativas se ubican bajo `os.UserConfigDir()/gitmoji/config.json`;
la importación del perfil Node es explícita y conserva el original. Las rutas
heredadas, defaults y política de hooks están fijados en COMPATIBILITY.
MEMORY enumera lo pendiente de verificar; objetivos de plataforma no equivalen
a soporte demostrado.
