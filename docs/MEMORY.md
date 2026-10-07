# Memoria de trabajo

Última actualización: 2026-10-06. Estado para retomar; contratos en
[CONTEXT](CONTEXT.md), [PLAN](PLAN.md) y [COMPATIBILITY](COMPATIBILITY.md).

## Estado verificado

- Repositorio/módulo: `github.com/Snah-s/commit_tool`; rama `migration/phase-0`.
  No se hicieron commits ni push durante las fases 0/1/2.
- `go.mod` mantiene `go 1.27.1`; dependencias fijadas en go.mod/go.sum:
  Huh 2.0.3, Bubble Tea 2.0.2, Bubbles 2.0.0 y helpers de terminal/ANSI.
  creack/pty se usa solo en pruebas; cancelreader cancela lectura accesible.
- Fase 0 completada: inventario estático, matriz de compatibilidad, defaults,
  precedencia, rutas, cancelación, hooks y cambios intencionales documentados.
- CLI de referencia: gitmoji-cli v9.7.0, commit
  `c4b0e56adea61c9e279b18d16cd46a837dd4bd55`. Fuentes consultadas por archivo;
  no se clonó su árbol ni se incorporó tooling Node.
- Rutas heredadas derivadas de conf 13.1.0
  (`657ab92b31396c5e29f64a3da9beb8ce0f097e70`) y env-paths 3.0.0
  (`f1729272888f45f6584e74dc4d0af3aecba9e7e8`). No validadas aún en ejecución.
- Assets en `internal/catalog/assets/`: catálogo original, clasificación local
  y licencia MIT. Origen gitmoji `9af0c0be38bc6a08561cedccc536674d4b1d7f0f`.
  Hay 75 entradas, 69 códigos clasificados, 81 asociaciones y seis excepciones
  documentadas disponibles solo en emoji. LICENSE raíz conserva el aviso upstream.
- Fase 1 completada localmente en Linux amd64: `cmd/gitmoji` ofrece `prototype`,
  `internal/ui` contiene formulario/preview y `internal/catalog` carga assets con embed.
  Los tres modos, búsqueda, contador, cancelación, accesibilidad y terminal de
  hook se probaron; guía en [PROTOTYPE](PROTOTYPE.md).
- Fase 2 completada localmente: `internal/commit` concentra constructor, preview,
  validación y parser de prefijos Unicode/shortcode para los tres modos.
  Conserva cuerpo, indentación, trailers y mayúsculas técnicas; cuenta puntos
  de código y avisa sobre 72. No hay formatter duplicado en UI.
- `internal/cli` reconoce comandos/aliases, flags intercalados, queries múltiples,
  contradicciones y valores explícitos vacíos. `cmd/gitmoji` despacha ayuda/versión,
  prepara mensajes sin TTY con datos completos y retorna errores diferenciados.
  `--format` omite el selector; otros flags precargan campos editables. Guía: [CLI](CLI.md).
- `commit` y `prototype` solo devuelven mensajes; no modifican archivos ni staging
  y no crean commits. Hook recupera campos y conserva cuerpo, permite revisar
  conversión entre modos y omite títulos ya válidos; nunca escribe el archivo.
  Los tests hacen commits e instalan wrappers únicamente en repositorios temporales.
- No hay configuración/caché/HTTP, cliente Git, instalación de hooks ni workflows
  de producto. config/list/search/update reconocen argumentos y señalan fase 3
  pendiente; init/remove señalan fase 4. Shortcodes y capitalización desactivada
  están disponibles en el constructor; la CLI usa Unicode y default heredado
  de capitalización emoji hasta incorporar preferencias en fase 3.
- `.gitignore` permite versionar docs y excluye logs locales salvo `.gitkeep`.
  AGENTS raíz remite a `docs/AGENTS.md`; el plan original está en `docs/PLAN.md`.

## Fases

| Fase | Estado |
| --- | --- |
| 0 — Contrato de migración | Completada; contratos y recursos verificados estáticamente. |
| 1 — Prototipo de interfaz y terminal | Completada y verificada en Linux amd64; otras plataformas pendientes. |
| 2 — Modelo de commit y CLI | Completada y verificada en Linux amd64; efectos Git pendientes de fase 4. |
| 3 — Configuración y catálogo | Pendiente. |
| 4 — Integración Git y hooks | Pendiente. |
| 5 — Compatibilidad y distribución | Pendiente. |
| 6 — Retirada del tooling Node | Pendiente; este checkout no contiene tooling Node que retirar. |

La fase 6 aplica únicamente a recursos heredados realmente incorporados;
no crear archivos Node para luego eliminarlos.

## Decisiones cerradas y pendientes

El usuario confirmó default `standard`, híbrido `feat(alcance): ✨ descripción`,
ejecutable `gitmoji` y objetivos Linux/macOS amd64/arm64. Windows amd64 queda
condicionado a validar hooks. También confirmó versionar documentos y mantener
logs locales. Los objetivos de plataforma no acreditan soporte demostrado.

Las preferencias nativas usarán `os.UserConfigDir()/gitmoji/config.json`;
importación del perfil Node explícita y conservando el original. Un perfil
heredado sin modo se importa como emoji. Detalles de rutas/defaults y política
de hooks cerrados en COMPATIBILITY.

Pendientes de las fases siguientes:

- Mantener Huh dentro del modelo Bubble Tea elegido: preview reactivo y filtrado
  síncrono requieren el wrapper. El driver accesible usa campos Huh por separado
  para respetar condiciones, errores y EOF; no desarrollar otra UI en paralelo.
- Implementar configuración, validación de catálogo/caché e importación; conectar
  preferencias al constructor/preview compartidos y probar efectos Git en fase 4.
- Validar hooks e importación en las plataformas objetivo; comprobar firma,
  worktrees, hooksPath y conservación del mensaje con Git real.
- Fijar versión nativa, canales de distribución y guía de retorno en fase 5;
  incluir los avisos de licencia en artefactos de release.

## Evidencia de cierre

Checks locales del 2026-10-06:

| Comprobación | Resultado |
| --- | --- |
| `go version` | `go1.27.1-X:nodwarf5 linux/amd64`. |
| `git --version` | `git version 2.56.0`. |
| `go list -m` | Reconoce `github.com/Snah-s/commit_tool`. |
| jq: catálogo | 75 entradas; campos requeridos y códigos válidos; code/name únicos. |
| jq: clasificación | Seis categorías no vacías; referencias válidas; 69 códigos, 81 asociaciones y seis excepciones exactas. |
| SHA256 | Assets y licencias coinciden con los hashes registrados en COMPATIBILITY. |
| Ignorados Git | Logs excluidos; .gitkeep exceptuado y documentos principales disponibles para versionar. |
| Documentación | Enlaces locales y diff revisados, sin errores de whitespace; contratos y tabla de modelos conservados. |
| `go vet ./...` | Pasa. |
| `go test ./...` y `go test -race ./...` | Pasan; formatos/parser, argumentos, procesos sin TTY y UI/PTY con Git real temporal. |
| `go build -o /tmp/gitmoji ./cmd/gitmoji` | Pasa; binario de fase 2 compilado. |
| Artefacto sin Git/Node | Ayuda, versión y preparación standard/hybrid pasan en `/tmp` con PATH sin Git/Node. |
| Regresión de `--format` | Prueba PTY repetida ocho veces; no muestra selector omitido ni pierde defaults/cuerpo. |

Esto acredita fases 0/1/2 en el entorno local, sin acreditar efectos Git de
producto ni soporte nativo macOS/Windows. PTY de 40×15, streams y restauración verificados
tras finalizar, cancelar y EOF; sin entrada a pantalla alternativa ni borrado
de scrollback. Hook sin TTY conserva archivo y sale 0; cliente sin TTY prepara
con argumentos completos o sale 2 por datos faltantes/inválidos.
Los registros de extracción y checks están en `docs/agent-logs/` y son locales;
los commits de origen y hashes relevantes también constan en COMPATIBILITY.

## Siguiente paso

Comenzar fase 3 de PLAN: configuración JSON validada y precedencia, importación
explícita de preferencias, caché validada, list/search/update y actualización
HTTP segura. Conectar modo, emojiFormat y capitalizeTitle al constructor/preview
existentes; no duplicar formatos ni adelantar efectos Git de fase 4.
