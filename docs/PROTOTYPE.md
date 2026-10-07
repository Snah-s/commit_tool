# Prototipo de interfaz — fase 1

El prototipo recopila respuestas, muestra el mensaje y lo devuelve en stdout
solo al finalizar. No ejecuta `git commit`, cambia staging, instala hooks,
guarda preferencias ni modifica archivos de mensajes. La [CLI de fase 2](CLI.md)
reutiliza este formulario; configuración/caché e integración Git siguen pendientes.

## Ejecutar

Desde un terminal interactivo:

```sh
go run ./cmd/gitmoji prototype
go run ./cmd/gitmoji prototype --accessible
```

También se admite `ACCESSIBLE=1`. Ayuda inicial: `go run ./cmd/gitmoji --help`.
Para compilar sin ejecutar el formulario:

```sh
go build -o /tmp/gitmoji ./cmd/gitmoji
```

El flujo usa `standard` inicialmente y permite elegir emoji/hybrid, tipo,
emoji compatible, alcance, descripción y cuerpo. El preview cuenta puntos de
código Unicode del título completo; superar 72 genera un aviso, sin bloquear.
Los 75 emojis y su clasificación se incluyen en el binario y funcionan offline.

Flechas navegan; Tab/Enter avanzan; Shift+Tab retrocede. `/` busca por código
y descripción en el selector. Esc cierra la edición de búsqueda; fuera de ella
cancela. Ctrl+C cancela. En el cuerpo, Ctrl+J o Alt+Enter insertan una línea.
PgUp/PgDn desplazan el preview completo, incluidos párrafos y trailers.
La confirmación final empieza en Cancelar y requiere elegir Finalizar.

En modo accesible, los campos de Huh reciben respuestas de texto: modo, tipo
y código exacto del emoji; Enter conserva defaults válidos. El cuerpo se
conserva salvo reemplazo explícito; al reemplazarlo se introducen líneas hasta
`/fin`, manteniendo líneas vacías y espacios. EOF devuelve error y Ctrl+C cancela.

La UI utiliza stderr o el terminal de control; stdout recibe únicamente el
mensaje confirmado. Cliente sin stdin/stderr TTY y sin argumentos completos:
error de uso (2); con datos completos prepara el mensaje. Cancelación:
130 en cliente, 0 en hook. Un fallo operativo devuelve 1.

## Invocación de hook para el prototipo

```sh
/tmp/gitmoji prototype --hook /ruta/COMMIT_EDITMSG message
```

Recupera campos de prefijos reconocidos y conserva el cuerpo completo mediante
el parser de fase 2; comentarios y políticas Git completas quedan para fase 4. En Linux/macOS
abre `/dev/tty` para funcionar aunque Git redirija stdin. Orígenes commit/merge
se omiten; sin terminal de control conserva el archivo y sale 0. Windows no abre
prompts de hook todavía. El archivo original permanece intacto incluso al finalizar:
el comando solo devuelve un mensaje para inspeccionar la interfaz.

Las pruebas instalan su wrapper exclusivamente en repositorios temporales.
No usar este comando como hook de producto ni mezclarlo con el cliente heredado.

## Elección de componentes y límites

Versiones fijadas en go.mod/go.sum: Huh 2.0.3, Bubble Tea 2.0.2 y Bubbles 2.0.0.
Huh requiere Go 1.25.8; el módulo mantiene Go 1.27.1 y compila en el entorno actual.
Se reutilizan sus campos dentro de un único modelo Bubble Tea y el viewport de
Bubbles. No hay otro formulario reactivo personalizado.

La fuente de [Huh 2.0.3](https://github.com/charmbracelet/huh/tree/v2.0.3)
mostró que el hook de vista se aplica antes de sustituir Content, y que el
formulario accesible recorre grupos ocultos e ignora errores de campos.
El modelo mantiene preview y filtrado síncronos, sin callbacks que modifiquen
respuestas en goroutines. El driver accesible usa campos Huh secuenciales,
propaga EOF y cancela la lectura antes de devolver el terminal.

`internal/commit` contiene el constructor, validador y parser compartidos con
la CLI y el preview. La salida actual usa Unicode y capitalización heredada en
emoji; la selección desde preferencias se incorporará en fase 3. El constructor
admite shortcodes y la CLI ya prepara mensajes sin TTY con argumentos completos.
`internal/catalog/catalog.go` solo lee assets embebidos; aún no implementa caché,
HTTP ni búsqueda de los comandos list/search.

## Verificación

```sh
go vet ./...
go test ./...
go test -race ./...
go build -o /tmp/gitmoji ./cmd/gitmoji
```

Las pruebas PTY usan creack/pty únicamente en tests Linux/macOS; comprueban
terminal de 40×15, streams, finalización, Esc/Ctrl+C/EOF, restauración del estado,
ausencia de pantalla alternativa y de borrado de scrollback. Ejecutan un commit
Git temporal para comprobar el terminal de control y la conservación del mensaje.
Otras pruebas cubren los tres modos, búsqueda, invalidación al cambiar tipo,
contador Unicode y cuerpo con párrafos, indentación y trailers.

Ejecución verificada en Linux amd64. Las pruebas nativas de macOS y Windows,
firma y demás escenarios Git permanecen pendientes; no se anuncia soporte
basándose en una compilación cruzada. Estado para retomar: [MEMORY](MEMORY.md).
