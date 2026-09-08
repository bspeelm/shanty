# Where it puts things

Four directories. Nothing else, ever — not `~/.config/mpv`, not your shell
files, nothing.

| directory | what is in it | losing it costs |
|---|---|---|
| `~/.config/shanty/` | `config.toml` and `credentials.toml` | your setup; `shanty setup` rebuilds it |
| `~/.local/state/shanty/` | resume position, scrobble backlog | a little history *(unused so far — v0.2)* |
| `~/.cache/shanty/` | cover art | bandwidth *(unused so far — v0.3)* |
| `$XDG_RUNTIME_DIR/shanty/` | the mpv socket | nothing; it is gone at logout |

All four honour the XDG variables if you set them. On a system without
`XDG_RUNTIME_DIR`, the socket falls back to a directory under the cache — never
a shared `/tmp`, where a predictable socket name is one writable directory away
from somebody else's socket answering under ours.

## Removing it

```sh
shanty uninstall
```

It reads the same list, so it cannot fall behind. It leaves `~/.cache` and
`$XDG_RUNTIME_DIR` themselves — shanty had to create them on the way to its own
directories and they are not shanty's to delete, since something else may be
about to use them.

The binary is not removed. It is wherever you put it.

## Your mpv is not touched

shanty starts mpv with `--no-config` and explicit flags. Whatever you have in
`~/.config/mpv` is neither read nor written, and there is no way to pass extra
flags through — which is deliberate, because that is how the promise would get
broken one bug report at a time.
