# commit_tool

Migration of `gitmoji-cli` to a native Go CLI with `emoji`, `standard`, and
`hybrid` commit modes.

The minimum Go version is declared in [go.mod](go.mod).

Prepare a message: `go run ./cmd/gitmoji prototype --type feat --title "add search"`.
Create a commit from the index: `go run ./cmd/gitmoji commit --type feat --title "add search"`.

List or search the offline catalog with `list` and `search bug linter`.
Inspect preferences with `config --show`, or save a mode with
`config --set 'commitFormat="hybrid"'`. Usage: [CLI guide](docs/CLI.md).

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

The native CI matrix covers Linux and macOS on amd64/arm64 and an experimental
Windows amd64 client. Windows managed hook installation is explicitly disabled
until its controlling terminal is validated. Local execution has been verified on
Linux amd64; other targets require successful native CI before being advertised.
Installation, configuration migration, rollback and release gates:
[distribution guide](docs/RELEASE.md).

Based on gitmoji-cli and the gitmoji catalog by Carlos Cuesta, under the [MIT license](LICENSE).
Attribution and resource provenance are recorded in the compatibility contract.
