# Contrato de migración — fase 0

Fecha: 2026-10-06. Contrato para implementar y verificar la CLI nativa; la
inspección del upstream fue estática, sin ejecutar Node ni sus pruebas.
Los criterios de las fases posteriores permanecen en [PLAN](PLAN.md).

## Decisiones de producto confirmadas

El usuario aceptó en esta sesión: modo `standard` predeterminado, híbrido con
emoji después de `: `, ejecutable `gitmoji`, Linux/macOS amd64 y arm64 como
objetivo inicial y Windows amd64 condicionado a validar sus hooks.
El repositorio y módulo siguen siendo `github.com/Snah-s/commit_tool`.

`--format` prevalece sobre `commitFormat`; sin ambos se usa `standard`.
En UI, sin `--format`, el selector de tres modos empieza en esa elección
resuelta y permite cambiarla. Guardar una preferencia requiere elección explícita.

| Modo | Con alcance | Sin alcance |
| --- | --- | --- |
| emoji | `✨ (usuarios): agregar búsqueda` | `📝 actualizar documentación` |
| standard | `feat(usuarios): agregar búsqueda` | `docs: actualizar documentación` |
| hybrid | `feat(usuarios): ✨ agregar búsqueda` | `docs: 📝 actualizar documentación` |

Unicode y shortcode son representaciones de la misma entrada. En híbrido también
es válido `feat(usuarios): :sparkles: agregar búsqueda`. Reglas completas de
descripción, alcance, cuerpo y contador: PLAN 3; no se añaden tipos fuera de los seis.

## Inventario y cambios intencionales

Referencia CLI: [v9.7.0, commit c4b0e56](https://github.com/carloscuesta/gitmoji-cli/tree/c4b0e56adea61c9e279b18d16cd46a837dd4bd55).
La matriz refleja código leído, no resultados de ejecución de la versión Node.

| Interfaz / comportamiento observado | Contrato nativo |
| --- | --- |
| `commit`, `-c`, `--commit` | Mantener selección interactiva; añadir los tres modos. Sin TTY, exigir argumentos completos. |
| `config`, `-g`, `--config` | Editar preferencias globales y mostrar sobreescrituras de proyecto; importar preferencias heredadas solo por elección explícita. |
| `init`, `-i`, `--init` | Instalar wrapper que invoque el binario nativo; comprobar propiedad antes de escribir. |
| `remove`, `-r`, `--remove` | Eliminar únicamente un hook reconocido como propio. |
| `list`, `-l`, `--list` | Listar catálogo completo offline, incluidos códigos sin clasificación híbrida. |
| `search`, `-s`, `--search` | Mantener `gitmoji search bug linter` y `gitmoji bug linter -s`; búsquedas independientes por consulta, sobre nombre/descripción. Sin consultas, listar. |
| `update`, `-u`, `--update` | Actualización HTTP explícita; validar antes de reemplazar y comparar contenido por código, no cantidad ni identidad de objetos. Sin cambios es éxito. |
| `--help`, `-h`; `--version`, `-v` | Ayuda/versiones sin repositorio ni red. `gitmoji commit --help` muestra ayuda sin efectos. La versión nativa no se confunde con 9.7.0 upstream. |
| Sin argumentos / comando desconocido | Sin argumentos: ayuda y éxito. Desconocido: diagnóstico y error de uso, en lugar de éxito silencioso con ayuda. |
| `--hook archivo [origen] [objeto]`; comando interno `hook` | Conservar ambos accesos observados; parsear parámetros explícitos y reenviar los tres argumentos de Git. |
| `--title`, `--message`, `--scope` | Conservar defaults interactivos. `--title` significa descripción; conservar valores suministrados aunque su prompt esté oculto. |
| Nuevos `--format`, `--type`, `--emoji` | Modos cerrados; tipo de los seis; emoji por `code` exacto, p. ej. `:sparkles:`. Rechazar emoji en standard y tipo en emoji. |
| Flags de acción simultáneos | Permitir redundancia del mismo selector; rechazar acciones distintas o un comando explícito incompatible. No heredar el orden de propiedades JS. |
| Valores con espacios, Unicode o guion inicial | Admitir valores entre comillas y `--title=-valor`; `--` termina flags y permite consultas literales con nombres de comandos. |
| Título: solo no vacío; contador 48 sobre descripción | Validación acordada en PLAN 3; aviso 72 sobre título completo en puntos de código. Capitalización automática solo en emoji. |
| Cliente con `shell: true` y comillas añadidas | Argumentos Git separados con contenido literal, sin expansión ni comillas artificiales. |
| `autoAdd=false` / `true` | False mantiene index. True ejecuta `git add .` desde cwd y después commit del index; omitir `-a` para no añadir implícitamente cambios fuera de cwd. |
| Cliente bloqueado con hook propio | Mantener bloqueo inicial; nunca desactivar hooks para evitarlo. |
| Hook: cuerpo limitado a línea 3; salida de fallo anulada | Conservar todo el mensaje y propagar fallo antes de reemplazar el original. |
| `init`/`remove` escriben/borran sin verificar propiedad | Conflicto con hook ajeno: error y conservar archivo. Reconocer wrapper heredado solo si coincide exactamente con el template fijado. |
| Configuración local: raíz omitida, JSON inválido ignorado | Incluir raíz; objeto vacío válido significa defaults. Candidato presente ilegible o inválido produce diagnóstico y detiene efectos; no caer silenciosamente a otro perfil. |
| Primer arranque obtiene catálogo de red / update-notifier | Usar caché válida o catálogo embebido; no consultar actualizaciones automáticamente. |

En ejecución sin TTY, `commit` con descripción y selección completa constituye
una acción explícita: standard necesita tipo; emoji necesita código; hybrid
necesita ambos y compatibilidad entre ellos. Alcance/cuerpo son opcionales.
En TTY, los argumentos siguen siendo defaults del formulario y se muestra preview
antes de finalizar. Los requisitos faltantes no deben iniciar esperas invisibles.

Salida de datos de list/search/help/version en stdout; presentación y diagnósticos
en stderr. Esquema de salida: 0 éxito o hook omitido; 2 error de argumentos o
configuración; 1 error operativo; 130 cancelación del cliente. Propagar el estado
no nulo de Git cuando esté disponible, distinguiéndolo del error de uso de la CLI.

## Preferencias y rutas

Desde cwd hasta raíz inclusive, en cada directorio: `package.json` con clave
`gitmoji`, después `.gitmojirc.json`. Archivo ausente o manifest sin esa clave
permite seguir buscando. La primera configuración encontrada debe ser un objeto
JSON con tipos válidos; `{}` selecciona defaults, sin mezclar preferencias globales.
Errores de lectura/JSON/tipos se informan con su ruta y detienen efectos.
Sin configuración de proyecto: global y después defaults.

| Preferencia | Default / tratamiento |
| --- | --- |
| `autoAdd` | false; conservar el booleano. |
| `scopePrompt` | false; conservar booleano o lista de strings, permitiendo omisión. |
| `messagePrompt` | true; ocultarlo no descarta cuerpo heredado ni suministrado. |
| `emojiFormat` | `emoji`; alternativas heredadas exactas `emoji` y `code`. |
| `capitalizeTitle` | true en emoji; sin efecto en standard/hybrid. |
| `gitmojisUrl` | `https://gitmoji.dev/api/gitmojis`; validar HTTP(S), estado y esquema. |
| `commitFormat` | `standard` en instalaciones nuevas; enum emoji/standard/hybrid. |

Upstream fija `conf@13.1.0` y resuelve `env-paths@3.0.0` en yarn.lock.
`Conf({projectName: 'gitmoji'})` usa sufijo `nodejs` y archivo `config.json`.
Las rutas heredadas se derivan de sus fuentes; falta validar importación en
ejecución sobre cada plataforma durante las fases 3 y 5.

| Sistema | Ruta heredada | Ruta nativa |
| --- | --- | --- |
| Linux | `$XDG_CONFIG_HOME/gitmoji-nodejs/config.json`, o `$HOME/.config/gitmoji-nodejs/config.json` | `$XDG_CONFIG_HOME/gitmoji/config.json`, o `$HOME/.config/gitmoji/config.json` |
| macOS | `$HOME/Library/Preferences/gitmoji-nodejs/config.json` | `$HOME/Library/Application Support/gitmoji/config.json` |
| Windows | `%APPDATA%/gitmoji-nodejs/Config/config.json`; sin APPDATA, `$HOME/AppData/Roaming/...` | `%APPDATA%/gitmoji/config.json` |

La ruta nativa es `os.UserConfigDir()` + `gitmoji/config.json`, conforme a la
[API oficial](https://pkg.go.dev/os#UserConfigDir). Un directorio no determinable
o XDG relativo produce error; no inventar fallback en cwd.

`config` ofrece importación explícita de JSON heredado cuando no existe el archivo
nativo: mostrar origen y valores, validar y escribir solo el destino. No invocar
Node ni modificar/eliminar el original. Preservar campos desconocidos sin darles
efecto. Al importar un perfil sin `commitFormat`, usar `emoji` para mantener su
formato previo; cambiar a standard requiere elección explícita del usuario.

La caché conserva `~/.gitmoji/gitmojis.json` y su array de entradas. El asset
oficial conserva el objeto raíz `gitmojis`; el lector distingue ambas formas.
Validar antes de usar/reemplazar caché; si es corrupta, avisar y usar el catálogo
embebido. Conservar el archivo previo si falla una actualización.

## Política de hooks y mensajes

El wrapper nuevo contiene marcador `gitmoji-native managed hook v1` y un template
reconocible. `init` registra ruta absoluta del ejecutable; si cambia la instalación,
repetir init solo sobre hook propio. Reenviar argumentos entre comillas y consultar
a Git rutas, hooksPath y worktrees; no asumir `.git/hooks` ni PATH compartido con Node.
El acceso a terminal se resuelve y verifica en fase 1, separado por plataforma.

La [documentación de Git](https://git-scm.com/docs/githooks#_prepare_commit_msg)
define archivo, origen y objeto opcional; un error del hook cancela el commit.
La política nativa queda fijada así:

| Caso | Resultado del hook |
| --- | --- |
| Origen `commit` (incluye amend/-c/-C), `merge` o rebase activo | Conservar archivo completo y salir 0 sin prompts, por compatibilidad inicial. |
| Sin terminal interactivo | Conservar archivo y salir 0; no es validación obligatoria de commits. |
| Título ya válido para el modo resuelto | Conservar mensaje completo y salir 0; un emoji cualquiera no basta para reconocer el modo. |
| Origen `message`, `template`, `squash` o no indicado, con TTY y título pendiente | Recuperar descripción/cuerpo y abrir formulario; confirmar conversión sin duplicar prefijos. |
| Cancelación de UI | Conservar archivo original y salir 0. |
| Argumentos/lectura inválidos o fallo de escritura | Salir con error; conservar original. No encubrir fallos como cancelación. |

Respetar `core.commentChar`, limpieza de templates y trailers. No filtrar todas
las líneas `#` indiscriminadamente. Construir y validar el mensaje completo antes
de sustituirlo, con reemplazo seguro. La detección real de estos casos y la firma
interactiva se prueban en repositorios temporales durante fase 4.

## Recursos, metadatos y atribución

| Fuente fijada | Recursos / evidencia |
| --- | --- |
| gitmoji-cli `c4b0e56adea61c9e279b18d16cd46a837dd4bd55` (v9.7.0) | Manifest, CLI, configuración, prompts, cliente, hook, caché, búsqueda, fixtures de configuración y workflows consultados por archivo. LICENSE integrado en raíz sin modificar. |
| conf `657ab92b31396c5e29f64a3da9beb8ce0f097e70` (13.1.0) | [source/index.ts](https://raw.githubusercontent.com/sindresorhus/conf/657ab92b31396c5e29f64a3da9beb8ce0f097e70/source/index.ts), solo lectura para defaults/rutas. |
| env-paths `f1729272888f45f6584e74dc4d0af3aecba9e7e8` (3.0.0) | [index.js](https://raw.githubusercontent.com/sindresorhus/env-paths/f1729272888f45f6584e74dc4d0af3aecba9e7e8/index.js), solo lectura para rutas por plataforma. |
| gitmoji `9af0c0be38bc6a08561cedccc536674d4b1d7f0f` | [Catálogo original](https://raw.githubusercontent.com/carloscuesta/gitmoji/9af0c0be38bc6a08561cedccc536674d4b1d7f0f/packages/gitmojis/src/gitmojis.json) y [LICENSE](https://raw.githubusercontent.com/carloscuesta/gitmoji/9af0c0be38bc6a08561cedccc536674d4b1d7f0f/LICENSE), integrados en `internal/catalog/assets/`. |

No se copian JS/TS, workflows Node ni yarn.lock al producto; se consultaron en
temporales. Registros detallados de extracción y revisión: `docs/agent-logs/`.
Los contratos se versionan; los logs permanecen locales por elección del usuario.

Metadatos preservados: upstream `gitmoji-cli` 9.7.0, autor Carlos Cuesta,
licencia MIT y copyright 2016–2022 Carlos Cuesta. El proyecto nativo mantiene su
repositorio propio; su versión y canales de release se fijan en fase 5. La
licencia del catálogo debe acompañar cada artefacto que lo distribuya.

| Recurso integrado | SHA256 |
| --- | --- |
| `LICENSE` y `internal/catalog/assets/LICENSE` | `463f3ada26f78ec3b566ce12fa09982e60e5f8c06e6e8dec1f32b559c4f4d5a6` |
| `internal/catalog/assets/gitmojis.json` | `b7ef2d4879d13ee7e7f8177784ffdb000082d1f22f42617f47fbbb7cadf23167` |
| `internal/catalog/assets/commit-emojis.json` | `430cb13497fb76658c34d8402fc1603d432fdaa2910804f693bc82ecc0b3403d` |

Se revisaron las 75 entradas: 69 códigos clasificados y 81 asociaciones bajo
seis tipos; 12 códigos tienen más de una asociación semántica. Las seis excepciones
son `:construction:`, `:poop:`, `:beers:`, `:rewind:`, `:boom:` y `:alembic:`.
Siguen disponibles en emoji; en híbrido indicar falta de clasificación y ofrecer
códigos clasificados. No asociarlos arbitrariamente a todos los tipos.

El ejemplo anterior `📚` / `:books:` no existe en el catálogo fijado; se corrigió
a `📝` / `:memo:`. Todos los códigos de ejemplo de PLAN 3.3 sí existen.

## Evidencia y salida de fase 0

Se contrastaron los contratos con las fuentes fijadas y se verificaron con jq
el esquema, identificadores únicos, seis categorías no vacías, referencias válidas
y cobertura exacta con las seis excepciones. SHA256 verifica los recursos integrados.
No hay aún fuentes Go ni pruebas de ejecución de CLI, hooks o terminal: pertenecen
a las fases siguientes. Fase 1 debe probar Huh, controlador de terminal y preview
antes de cerrar versiones/componentes de UI.
