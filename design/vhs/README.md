# Demo recordings

Scripts for [VHS](https://github.com/charmbracelet/vhs) that record Qrypt on its sample data (`--preview`), for visual review of a design change and for the README.

| Script | Shows |
| --- | --- |
| `preview-wide.tape` | Wide mode: tree, items, inspector, reveal, help |
| `preview-narrow.tape` | Narrow flow: first screen with the logo, spaces, items, help |

```
go build -o qrypt ./cmd/qrypt
vhs design/vhs/preview-wide.tape
vhs design/vhs/preview-narrow.tape
```

Recordings are written to `design/vhs/out/`, which is git-ignored. The scripts only ever run `--preview`, so no real identity or vault is opened. They expect a Nerd Font to be installed for the `nerd` icon set; the default `emoji` set needs none.

> **Status:** the scripts have not been recorded yet. Check the key sequence against the current screens the first time you run them.
