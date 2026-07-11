# Gloss

Async annotation and review for AI-generated content.

Gloss is a single binary: run `gloss .` against any directory and it
boots a localhost web UI plus an MCP server, backed by SQLite. Read
AI-generated files in your own time, comment on specific lines and
regions, and feed those threads back to the AI via MCP. No auth, no
account, no internet connection required.

## Status

Pre-release — milestone 4 (web server shell) of 11. See
[PLAN.md](PLAN.md) for the delivery breakdown. `gloss .` now boots a
read-only file-browser at `http://localhost:4747`; annotation and MCP
are not implemented yet.

## Building from source

Requires Go 1.24+.

```sh
make build
./gloss .
```

## License

[MIT](LICENSE)
