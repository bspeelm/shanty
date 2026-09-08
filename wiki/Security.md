# Security

shanty holds a credential for your server for as long as it is running. This
page describes what it does with that credential, what it assumes about your
server, and what it connects to.

## Your password

`shanty setup` asks for your password once and does not save it. What it writes
instead is the scrambled value that Subsonic servers accept in place of a
password, together with the random string used to produce it. That pair
authenticates against your server and cannot be used anywhere else, so someone
who copies the file can play your music but cannot sign in to your other
accounts.

An API key is better still, because you can revoke one from your server's web
interface without changing your password or affecting anything else. `shanty
doctor` tells you when your server supports API keys and you are not using one.

If you write a plain password into `credentials.toml` yourself, shanty will use
it. It will never write one there.

## The permissions on the credential file

`credentials.toml` must not be readable by other users of the machine. shanty
checks this every time it starts and refuses to run if the file is too
permissive, printing the `chmod` command that fixes it. It stops rather than
warning, because a warning printed once at startup is easy to miss.

## Other users of the same computer

On a shared machine, any user can list the processes that are running and see
the command-line arguments each was started with.

Music URLs from a Subsonic server contain your credential as a parameter. shanty
therefore never passes one as a command-line argument. It starts a single mpv
process and sends each track to it through a socket file, which lives in a
directory that only your user account can open. The URL exists in mpv's memory
and is not visible to anyone else on the system.

A session left running by `:headless` is another shanty process, started the
same way and holding nothing in its arguments either. The queue reaches it as a
list of track identifiers; it looks up your credential itself. It is commanded
through a second socket in the same directory, which accepts a fixed list of
seven things and cannot be asked to open a file or run a program.

## What shanty keeps about your listening

A play your server would not accept is kept on disk until it will, in a file
only your account can read. It holds the track identifier and the time, which
is what the server is told when it is reachable again. Nothing is kept once the
report has been accepted.

## Revoking a credential does not stop a session

If you revoke an API key or change your password while a session is playing,
that session carries on with the credential it started with. It will play what
is already queued and its reports to your server will start failing, where
nobody is watching to see them fail.

It cannot last longer than the queue it was given, because a session ends when
its music does. To be certain, run `shanty stop`. `shanty doctor` says whether
one is running.

## What shanty assumes about your server

shanty treats every response from your server as untrusted, on the basis that a
server can be compromised or simply faulty.

Responses are read up to a fixed size limit, so a server that keeps sending
data cannot exhaust your machine's memory. The data itself is parsed by a
single routine that is tested against malformed and truncated input.

Artist names, album titles and track titles are stripped of any characters that
a terminal would interpret as an instruction before they are displayed.
Terminals treat certain sequences of characters as commands — to move the
cursor, clear the screen, or in some cases to send text back as though you had
typed it — and a track title is not allowed to become one of those.

Audio never passes through shanty. mpv connects to your server and fetches it
directly.

## What shanty connects to

Your server, and nothing else.

shanty does not check for updates, report errors or crashes, collect usage
data, or contact any third-party service. Plays are reported to your own
server's endpoint. If you want them forwarded to Last.fm or ListenBrainz, that
is configured on the server you already administer, so those credentials stay
in one place rather than being copied to every machine you install shanty on.

## Certificate verification

Certificate verification cannot be turned off. There is no command-line flag,
configuration setting, or environment variable that disables it.

If your server uses a self-signed certificate, add that certificate to your
system's trust store. That is the mechanism your operating system already
provides for this, and it applies to every program rather than only to shanty.

## Verifying what you downloaded

Every release is published with a file of checksums and a signature produced by
the automated build that created it.

The install script checks the download against the checksums every time. Adding
`--verify` also checks the signature, which confirms the file came from this
project's build process. The script tells you which of the two checks it
performed.
