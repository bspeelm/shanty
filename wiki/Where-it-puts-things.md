# Where it puts things

Four directories, and nothing outside them.

| directory | contents | if you delete it |
|---|---|---|
| `~/.config/shanty/` | your server address and credential | run `shanty setup` again |
| `~/.local/state/shanty/` | resume position and pending scrobbles | you lose a little history *(not used yet)* |
| `~/.cache/shanty/` | cover art | it downloads again *(not used yet)* |
| `$XDG_RUNTIME_DIR/shanty/` | the connection to mpv while it is playing | nothing; it goes at logout |

All four follow the `XDG_` environment variables if you set them. On a system
without `XDG_RUNTIME_DIR`, the connection to mpv lives under the cache
directory instead — never in a shared temporary directory, where another user
could get there first.

## What it does not touch

Your mpv configuration. shanty starts mpv with its own settings and ignores
whatever is in `~/.config/mpv`, so nothing you have set up there changes and
nothing shanty does can break it.

Your shell files, your other programs' configuration, and anything else on the
machine.

## Removing it

```sh
shanty uninstall
```

It prints what it removed and what was not there. `~/.cache` and the runtime
directory themselves stay — shanty made them on the way to its own folders but
they belong to the system, and other programs use them.

The binary is not removed. Delete it from wherever you installed it, usually
`~/.local/bin/shanty`.
