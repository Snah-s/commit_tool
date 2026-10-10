# commit_tool

Migration of `gitmoji-cli` to a native Go CLI with `emoji`, `standard`, and
`hybrid` commit modes.

From an extracted package, run the standalone executable:

```sh
./gitmoji --help
./gitmoji prototype --format standard --type feat --title "add search"
```

Go is required only to build from a source checkout; its minimum version is
declared in [go.mod](go.mod). The source commands below require that checkout,
which is not included in the binary package.

Prepare a message: `go run ./cmd/gitmoji prototype --type feat --title "add search"`.
Create a commit from the index: `go run ./cmd/gitmoji commit --type feat --title "add search"`.

List or search the offline catalog with `list` and `search bug linter`.
Inspect preferences with `config --show`, or save a mode with
`config --set 'commitFormat="hybrid"'`. `update` refreshes the catalog only when
requested; normal use is offline. Run `gitmoji --help` for commands and flags.

`standard` is the default. Use `--format emoji` or `--format hybrid` to select
another mode, `--title` for the description, `--scope` for an optional scope,
and `--message` for the body. Types are `feat`, `fix`, `docs`, `refactor`, `test`
and `chore`. In a terminal, review the prompts and confirm before committing.
The body prompt accepts one optional line; leaving it empty omits the body.
Without a terminal, complete arguments authorize the commit directly.
The index is preserved unless you explicitly enable `autoAdd`.

Compile a persistent executable before installing a hook:

```sh
go build -o ./bin/gitmoji ./cmd/gitmoji
./bin/gitmoji init
```

Hooks record the absolute executable path. Re-run `init` after moving the binary;
`remove` deletes only a recognized managed hook. The commit client is blocked
while that hook is active; use `git commit` in that workflow.

Build a candidate package, including dependency licenses and a SHA256 checksum:

```sh
sh scripts/package.sh 0.1.0-rc.1
```

Packages contain a standalone executable; Git is required for commits and hooks.
Node, npm, Yarn and npx are not required. `--version` reports the target, toolchain,
revision and commit date; packages built from an uncommitted checkout are marked dirty.

The same Go implementation serves Linux, macOS and Windows, with platform-specific
terminal access. CI currently targets Linux amd64; other native jobs are deferred.
Windows controlling terminal access still needs
implementation and validation; managed hook installation is disabled. Local
execution has been verified on Linux amd64; other targets require successful
native CI before being advertised.

## Install a Linux package

Use a package that has passed native execution checks. From its download directory:

```sh
sha256sum -c gitmoji_0.1.0-rc.1_linux_amd64.tar.gz.sha256
tar -xzf gitmoji_0.1.0-rc.1_linux_amd64.tar.gz
mkdir -p "$HOME/.local/bin"
cp gitmoji_0.1.0-rc.1_linux_amd64/gitmoji "$HOME/.local/bin/gitmoji"
chmod 755 "$HOME/.local/bin/gitmoji"
"$HOME/.local/bin/gitmoji" --version
command -v gitmoji
```

Add `$HOME/.local/bin` to PATH if needed. The checksum checks integrity against
the supplied checksum file; verify the download source too. Keep the complete
package and license notices when redistributing. Candidates are CI artifacts;
publication requires accepted native checks and an authorized tag/release.

## Preferences and return to the previous version

`config --show` reports effective preferences and their source. The nearest
project configuration wins: the `gitmoji` key in `package.json`, then
`.gitmojirc.json` in each directory up to the root. Otherwise, global preferences
under the user's configuration directory apply. Reading JSON requires no Node.

Back up the previous executable, hook and profile before trying the candidate.
Import a legacy profile explicitly with `gitmoji config --import-legacy FILE`.
Import preserves the original and refuses to overwrite a native profile;
legacy profiles without a commit format use `emoji`, while new profiles use
`standard`. Native search does not reproduce the previous Fuse.js fuzzy ranking.

To return, run the native executable by absolute path with `remove` in each
repository where it installed a hook. Restore the previous executable in PATH
and the backed-up hook only if its destination is absent; integrate a modified
hook manually. The original profile remains available. Keep the previous
installation while evaluating the candidate.

## Development

The source checkout requires Go and Git; packaging also uses a POSIX shell,
tar/gzip and common file utilities.
Run `go vet ./...`, `go test ./...` and `go test -race ./...` on Linux before
building a candidate. CI runs those checks and tests the extracted executable.
Project development, testing and packaging require no Node tooling.
Local planning documents under `docs/` are excluded from Git and binary packages.

Based on gitmoji-cli and the gitmoji catalog by Carlos Cuesta, under the [MIT license](LICENSE).
Packages include the project, catalog, Go and linked-module license notices.
