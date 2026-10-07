# CLI — fase 2

El ejecutable prepara mensajes en `standard`, `emoji` e `hybrid`. En esta fase
devuelve el mensaje confirmado por stdout y los avisos por stderr; la UI usa
stderr o el terminal de control. Todavía no ejecuta Git, modifica staging,
escribe mensajes de hook ni guarda preferencias.

## Ejecutar

```sh
go run ./cmd/gitmoji --help
go run ./cmd/gitmoji --version
go run ./cmd/gitmoji commit
go run ./cmd/gitmoji commit --format hybrid --type feat --emoji :sparkles: --title "agregar búsqueda"
```

`standard` es el default. En un terminal, `--format` omite el selector de modo;
los otros flags precargan campos editables. `--title` es la descripción sin
prefijos; `--message` es el cuerpo completo. Alcance opcional mediante `--scope`.
El formulario, las teclas y `--accessible` se describen en [PROTOTYPE](PROTOTYPE.md).
El alias `prototype` conserva acceso al mismo flujo de preparación.

Sin stdin/stderr TTY, la selección y la descripción deben estar completas:

```sh
go run ./cmd/gitmoji -c --type docs --scope cli --title "actualizar README"
go run ./cmd/gitmoji -c --format emoji --emoji :memo: --title "actualizar README"
go run ./cmd/gitmoji -c --format hybrid --type docs --emoji :memo: --title "actualizar README"
```

Salidas: `docs(cli): actualizar README`, `📝 Actualizar README` y
`docs: 📝 actualizar README`. La CLI emite Unicode y capitaliza únicamente
la descripción del modo emoji. El constructor admite también shortcodes y
capitalización desactivada; su selección desde preferencias corresponde a fase 3.

Se preservan mayúsculas técnicas, comillas, `$`, backticks, Unicode, párrafos,
indentación y trailers. La salida tiene un salto de línea final si faltaba;
no añade otro cuando el cuerpo ya termina en salto. Títulos de más de 72 puntos
de código Unicode generan aviso, sin rechazo.

Flags intercalados y aliases cortos/largos se admiten; repetir un valor usa el
último. `--title=-valor` evita ambigüedad con un guion inicial. Acciones distintas
como `commit --list` se rechazan. Las consultas se conservan por separado:
`search bug linter`, `bug linter -s` y `-s -- commit --literal`.

## Comandos y salidas

| Acción | Alias | Estado |
| --- | --- | --- |
| `commit` | `-c`, `--commit` | Preparar y devolver mensaje. |
| `config` | `-g`, `--config` | Operación pendiente de fase 3. |
| `list` | `-l`, `--list` | Operación pendiente de fase 3. |
| `search` | `-s`, `--search` | Argumentos disponibles; búsqueda pendiente de fase 3. |
| `update` | `-u`, `--update` | Operación pendiente de fase 3. |
| `init` | `-i`, `--init` | Instalación pendiente de fase 4. |
| `remove` | `-r`, `--remove` | Retirada pendiente de fase 4. |
| Ayuda | `-h`, `--help` | Sin argumentos también muestra ayuda. |
| Versión | `-v`, `--version` | Versión nativa de desarrollo. |

Ayuda y versión funcionan fuera de un repositorio, sin Git, Node ni red.
Código 0 indica éxito; 2, uso inválido o datos incompletos sin TTY; 1, error
operativo o comando aún pendiente. Cancelar devuelve 130 en cliente y 0 en hook.

## Preparar desde un mensaje existente

```sh
go run ./cmd/gitmoji hook "/ruta con espacios/COMMIT_EDITMSG" message --format hybrid --type docs
```

También se admite `--hook ARCHIVO [origen [objeto]]`. El parser reconoce tipo,
alcance y prefijos Unicode/shortcode del catálogo; conserva la descripción y
el cuerpo. Al convertir entre modos evita reutilizar el prefijo como descripción.
Un título libre conserva su texto. Comentarios y políticas Git completas
(rebase, templates, squash, escritura segura) se resolverán en fase 4.

Con terminal de control, el formulario permite revisar la conversión. Orígenes
`commit`/`merge`, ausencia de terminal y títulos válidos del modo solicitado sin
campos reemplazados omiten el formulario. El archivo original siempre permanece
intacto en esta fase. No instalar esta invocación como hook de producto todavía.
