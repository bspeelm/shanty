# Where it puts things

shanty writes to four directories and nowhere else.

| Directory | What is in it | If you delete it |
|---|---|---|
| `~/.config/shanty/` | Your server address, username and credential | Run `shanty setup` to recreate it |
| `~/.local/state/shanty/` | Plays your server would not accept, kept until it will | You lose the listening history that had not been reported yet. |
| `~/.cache/shanty/` | Downloaded cover art | It is downloaded again when needed. *Not used yet.* |
| `$XDG_RUNTIME_DIR/shanty/` | The connection to mpv, and the one a session left by `:headless` is commanded through | Nothing. It is removed when you log out. |

All four honour the standard `XDG_` environment variables if you have set them.
The paths above are the defaults when you have not, on every system shanty runs
on. macOS is included: shanty puts its files in `~/.config` and `~/.cache` there
too, rather than under `~/Library`.

On systems that do not provide `XDG_RUNTIME_DIR`, the connection to mpv is
placed inside the cache directory instead. It is never placed in a shared
temporary directory such as `/tmp`, because another user of the machine could
create a file there first under the name shanty expects.

## What shanty does not touch

**Your mpv configuration.** shanty starts mpv with an explicit set of options
and instructs it to ignore its own configuration file. Nothing in
`~/.config/mpv` is read, and nothing shanty does can change it. There is also
no way to pass extra options through to mpv.

**Anything else on your system.** shanty does not modify shell configuration
files, install anything, or write outside the four directories above.

## Removing shanty

```sh
shanty uninstall
```

This deletes the four directories and prints which ones it removed and which
were already absent.

It leaves `~/.cache` and `$XDG_RUNTIME_DIR` themselves in place. shanty created
those on the way to its own directories, but they are shared with other
programs and are not shanty's to remove.

The shanty binary itself is not deleted. Remove it from wherever you installed
it, usually `~/.local/bin/shanty`.

## Plays your server did not take

If your server is unreachable or refuses a play report, shanty keeps the play
in `~/.local/state/shanty/plays.jsonl` and sends it the next time a report
works. The record carries the time you listened, so a history that catches up a
day later is not all dated to the moment it caught up.

Nothing accumulates once your server is answering: the file is removed when
there is nothing left waiting.

The file is one line of JSON per play, readable only by your account, and it
holds track identifiers and times rather than anything about you.

