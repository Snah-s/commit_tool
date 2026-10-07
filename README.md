# commit_tool

Migration of `gitmoji-cli` to a native Go CLI with `emoji`, `standard`, and
`hybrid` commit modes.

The minimum Go version is declared in [go.mod](go.mod).

Prepare a message: `go run ./cmd/gitmoji commit --type feat --title "add search"`.

Based on gitmoji-cli and the gitmoji catalog by Carlos Cuesta, under the [MIT license](LICENSE).
Attribution and resource provenance are recorded in the compatibility contract.
