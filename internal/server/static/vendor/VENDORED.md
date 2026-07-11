# Vendored assets

Checked in directly per CLAUDE.md ("no CDN, ever"). To update, fetch the
new version's npm tarball and re-copy the listed file.

| File | Package | Version | License |
|---|---|---|---|
| `htmx.min.js` | [htmx.org](https://www.npmjs.com/package/htmx.org) | 2.0.10 | 0BSD |
| `alpine.min.js` | [alpinejs](https://www.npmjs.com/package/alpinejs) | 3.15.12 | MIT |

`alpine.min.js` is Alpine's `dist/cdn.min.js` build — the self-contained
browser bundle meant for a plain `<script>` tag (auto-starts, no bundler
required), not `dist/module.esm.min.js`.
