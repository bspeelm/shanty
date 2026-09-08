# Security

shanty holds a credential for your server the whole time it is running. This is
what it does with it, and where it connects.

## Your password

`shanty setup` asks for it once and does not save it. What goes in the file is
the scrambled form Subsonic servers accept — useful against that one server,
useless anywhere else. If you write a plain password into the file yourself,
shanty will use it, but it will never put one there.

An API key is better again: it is a single string you can cancel from your
server without changing anything else. `shanty doctor` tells you when your
server offers them.

`credentials.toml` has to be readable only by you. If it is not, shanty stops
and prints the command to fix it rather than starting anyway.

## Other people on your machine

Anyone with an account on the same computer can list the programs that are
running and see the arguments they were started with. Music URLs from a
Subsonic server carry your credential in them, so shanty never puts one on a
command line: it starts mpv once and passes each track through a private
socket, which lives in a directory only you can open.

Nothing shanty stores or passes to mpv is visible to another user of the
machine.

## Your server

shanty assumes a server can be wrong, whether it has been broken into or is
simply misbehaving.

Everything it sends is checked before it is used. Replies have a size limit, so
a server cannot exhaust the machine's memory by answering forever. Track and
artist names have anything a terminal would act on removed before they are
drawn — terminals treat certain character sequences as instructions rather than
text, and a name is not allowed to become one.

Audio never passes through shanty at all. mpv fetches it directly.

## Connections

shanty connects to your server. That is the entire list.

No update checks, no error reporting, no telemetry, no third-party services.
Scrobbles go to your server's own endpoint — if you want them forwarded to
Last.fm or ListenBrainz, that is a setting on the server you already run,
rather than another password on every machine you install this on.

Certificate checking cannot be turned off. There is no flag, setting, or
environment variable for it. If you use a self-signed certificate, add it to
your system's trust store, which is the mechanism that already exists for that.

## Downloads

Every release comes with checksums and a signature produced by the build
itself. The installer checks the checksum every time and the signature when you
pass `--verify`, and tells you which of the two it did.
