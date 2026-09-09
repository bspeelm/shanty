# Commands

shanty has fifteen commands. Running `shanty` with no arguments browses and
plays; the rest are named. Seven of them command a session left running by
`:headless`, and are listed together at the end.

## `shanty`

Opens the browser and plays music. This is the command you will use almost all
of the time.

If shanty has not been configured yet, it runs the setup questions first. See
[Your first run](Your-first-run).

## `shanty setup`

Asks for a server address, a username, and a credential, then writes shanty's
two configuration files.

Use it when you want to point shanty at a different server, or to replace a
password with an API key. It overwrites both files, and it writes nothing
unless your server accepts the credential you gave it.

## `shanty doctor`

Examines your setup and reports on eleven things, from whether mpv is installed
to whether your server accepts your credential. Anything that is wrong is
listed together with the command or configuration change that fixes it. The
[The doctor](The-doctor) page explains each check.

```sh
shanty doctor        # human-readable report
shanty doctor -json  # the same report as JSON, for scripts
```

The command exits with a non-zero status if any check failed. Warnings do not
cause a non-zero exit, because they describe things you would probably want to
change rather than things that stop shanty working.

`shanty doctor` only inspects. It does not create, change, or delete anything.

## `shanty uninstall`

Deletes the directories shanty created and prints which ones it removed and
which were not present. The [Where it puts things](Where-it-puts-things) page
lists them.

If one of those paths is a symbolic link, shanty refuses to delete it and tells
you, rather than following the link and deleting something elsewhere.

It refuses while a session left by `:headless` is playing, because one of those
directories holds the connection that session is reached through. Run
`shanty stop` first.

It does not delete the shanty binary itself. That is wherever you installed it,
which is usually `~/.local/bin/shanty`.

## `shanty version`

Prints the version of the binary you are running. A build made from a clone of
the repository rather than from a release reports `dev`.

## `shanty completions`

Sets up tab completion, so pressing tab after `shanty` offers its commands
instead of listing the directory you are standing in.

Installing shanty runs this for you, so there is usually nothing to do. If you
want to run it yourself:

```sh
shanty completions install
```

It writes one file per shell you have, in the directory that shell already
reads completions from. Nothing is added to `~/.bashrc` or any other startup
file, and `shanty uninstall` removes the files again. Open a new terminal
afterwards.

A shell that is not on your machine gets no file. `zsh` reads completions only
from a directory owned by root, so `shanty completions zsh` writes the script
to the screen and you place it yourself:

```sh
shanty completions zsh | sudo tee /usr/share/zsh/site-functions/_shanty
```

The script is written from the list of commands shanty actually has, so it
cannot fall behind the program.

## `shanty help`

Lists the commands with a one-line description of each.

## Commanding a session

Typing `:headless` in the interface closes it and leaves the music playing.
These eight command what it leaves behind, and each works from any shell.

## `shanty status`

Says what is playing: the track, the artist, how far into it you are, the
volume, and which track of how many.

## `shanty play`

Resumes a session that is paused. It does nothing to one that is already
playing.

## `shanty pause`

Pauses a session that is playing. It does nothing to one that is already
paused.

Pausing and resuming are separate commands rather than one that toggles. In
the interface, `space` toggles because the screen tells you which way it will
go; a command typed in another shell has nothing to look at, and `shanty pause`
in a script must mean paused rather than "the other one".

## `shanty next`

Skips to the next track in the session's queue.

## `shanty prev`

Goes back to the previous track.

## `shanty vol 40`

Sets the volume to a number from 0 to 100. A number outside that range is
reported rather than rounded to the nearest end.

## `shanty seek 1:23`

Moves to a position in the current track, written as minutes and seconds or as
a number of seconds.

## `shanty stop`

Ends the session and the music with it. It also stops a player left behind by a
session that was killed, because what you want when you type it is silence.

Stopping when nothing is playing is not an error; it says so and exits without
complaint.

---

With no session running, all but `shanty stop` say so and name what to run instead.
Running `shanty` on its own returns the interface to a session that is playing.

