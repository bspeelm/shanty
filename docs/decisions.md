# Decisions

Numbered and append-only. A record is superseded or amended by a later record,
never edited away and never deleted. Titles state the decision rather than
naming a topic.

Refusals are recorded here as well as choices. A decision already declined is
settled, and a proposal to revisit it is answered with the record's number
rather than by arguing it again.

---

## ADR-001 — Threat model: what a stolen credential, another local user, a hostile server, and the network can each do

**Status:** accepted, before any code was written.

Four questions, in this order because each answer constrains the next. Every
"therefore" below corresponds to a test.

### What does a stolen configuration file give an attacker?

That depends entirely on what is in it, so shanty prefers credentials in one
order, and that order is the decision:

1. An **OpenSubsonic API key**. A single revocable string. The user cancels it
   from the server without changing anything else.
2. A **hashed token and salt**. Authenticates against this one server and is of
   no use anywhere else, because it is not the password.
3. A **password file** the user manages themselves, such as an `agenix` secret
   or a `pass(1)` entry.

A plain password is the fourth possibility. shanty reads one if the user wrote
it, but never writes one itself, and `doctor` names a better option whenever it
finds one.

Refusing to read a plain password would not protect anyone. It would only push
people towards a client that accepts one without comment. Refusing to write one
is a promise shanty can actually keep.

### What can another user of the same machine see?

Process arguments, the environment of any process shanty starts, files readable
by others, and anything in a shared temporary directory.

Therefore no credential appears in a child process's arguments or environment,
the credential file is `0600` and separate from the settings, and the socket
shanty uses to control mpv lives in a `0700` directory under
`XDG_RUNTIME_DIR`.

Stream URLs from a Subsonic server contain the credential as a query parameter.
Passing one to a media player as a command-line argument makes it visible to
every account on the machine through `ps`, which is why shanty passes tracks
over a socket instead.

### What can a compromised or faulty server do?

It chooses every byte shanty parses: JSON, cover art, lyrics, and stream data.

Therefore response parsing is fuzzed, image data is size-limited before it
reaches a renderer, response bodies are read through a capped reader, and a
string from the server never becomes a filesystem path except through one
sanitising function with its own tests.

The assumption is not that the server is hostile. It is that "my server" and
"some server" are the same code path, and only one of them is trustworthy.

### What can the network do?

Delay indefinitely, or lie.

Therefore every HTTP client has a timeout, and **certificate verification has
no way to be disabled** — not a flag, not a configuration key, not an
environment variable. A user with a self-signed certificate adds it to their
system trust store, which is the mechanism that already exists for this.

An `insecure = true` option would end up copied into setup guides and enabled
by people who did not know what it turned off. `make budgets` checks for
`InsecureSkipVerify` so it cannot be added by accident.

The cost is accepted: someone with a self-signed certificate who does not want
to touch their trust store will find shanty harder to use than a client that
offers the option.

---

## ADR-002 — The project is called shanty

**Status:** accepted.

A shanty is a rough hut and a song sung by people working. Both fit: something
small you built yourself, and music coming out of it. There is no conflicting
Go module and no conflicting binary in a normal `PATH`.

The name is recorded now rather than left open because it is not only a label.
It is the module path, the binary name, all four directories shanty writes to,
and the client identifier every Subsonic request carries — which servers log
and users filter their play history by.

Renaming after a release would move a user's configuration directory without
warning and separate their play history on the server from anything they could
find. Choosing now costs an afternoon. Deferring costs whoever installs early.

---

## ADR-003 — The interface is built with Bubble Tea, inside the module budget

**Status:** accepted.

Bubble Tea is the best-tested option for a terminal interface in Go, and
`teatest` provides golden-file rendering tests without building a test harness
first. Because every screen should produce a readable diff when its rendering
changes, that is worth more here than it would be in most projects.

The argument against is real. The Charm libraries are the largest single
dependency decision this project will make, and the module budget is 30 in
total. Bubble Tea together with Bubbles, Lipgloss and `teatest` was estimated
at 20 to 24 modules, leaving very little room for anything else, permanently.

That tension is resolved in favour of the budget. Bubble Tea is adopted, and 30
remains a hard ceiling rather than an opening position. The module count is
measured before the dependency is committed. If the full set does not fit,
Bubbles and Lipgloss are dropped and the list rendering is written by hand
against Bubble Tea's core. The budget is not raised to accommodate a
convenience.

---

## ADR-004 — Certificate verification cannot be disabled

**Status:** accepted. This is a standing refusal.

There is no flag, configuration key, environment variable, or build tag that
turns off certificate verification. ADR-001 gives the reasoning; this record
exists so the refusal has a number to answer with, because it will be asked
for.

A user with a self-signed certificate adds it to their system trust store. That
mechanism already exists, every other program on the machine already respects
it, and it solves the problem once rather than per-application.

An option to skip verification would be copied into setup guides and enabled by
people who did not know what it disabled.

The cost is accepted and is not trivial: someone with a self-signed certificate
who does not want to modify their trust store will find shanty harder to use
than a client that offers the option. `make budgets` checks for
`InsecureSkipVerify` so the option cannot appear by accident, and `doctor`
should suggest the trust store when it sees a certificate error.

---

## ADR-005 — Play reporting goes to the user's own server and nowhere else

**Status:** accepted. This is a standing refusal.

shanty reports plays to the Subsonic `scrobble` endpoint on the configured
server. It does not connect to Last.fm, ListenBrainz, Libre.fm, or any other
service.

The reason is where credentials end up, not a preference about scope. A
Subsonic server can already forward plays onward when its administrator
configures it to, and that administrator is the same person running this
client. Adding direct scrobbling would mean a second set of credentials, stored
on every machine shanty is installed on, for a service the server can already
reach. That increases the number of places a secret lives in order to save one
configuration step somewhere the user already has an account.

It also keeps a simpler property true: shanty connects to exactly one host, so
the answer to "what does this program contact" is the same as "what did I
configure".

"Add Last.fm support" is expected to be the most common request. It is answered
with this number.

---

## ADR-007 — Navidrome is the supported server; others are untested

**Status:** accepted.

shanty declares Subsonic API version `1.16.1` and targets Navidrome from the
current release back two minor versions.

OpenSubsonic extensions, API keys in particular, are detected rather than
assumed. The client asks the server what it supports and falls back to a token
and salt when the answer is no.

Other Subsonic-compatible servers may well work and are not tested. The
integration test runs one server image, so Navidrome is the only compatibility
claim with evidence behind it. Claiming support for gonic, Airsonic or Ampache
without a test that exercises them would be an unsupported claim, and every
user-facing claim in this project either names the test that proves it or is
removed. A second server image can be added later, and the claim can be added
at the same time.

---

## ADR-008 — The licence is MIT

**Status:** accepted.

Nothing in shanty is derived from a copyleft source, so there is no obligation
pulling in another direction.

MIT keeps the door open for anyone who wants to take the fake Subsonic server
or the isolation test suite and use them in their own client, which would be a
good outcome for both.

---

## ADR-009 — Configuration is TOML, parsed by pelletier/go-toml/v2

**Status:** accepted, before the dependency was added.

The strongest argument against is that this buys very little. Both files are a
handful of `key = "value"` lines. Sixty lines of string splitting would read
them, cost no dependency, and never need a security advisory read. For parsing
what shanty itself writes, that is entirely true.

It is not what the parser is for. Both files are meant to be edited by hand: a
`password_file` pointing at an `agenix` secret or a `pass(1)` entry is
something a person types. The parser's real job is reading what somebody wrote
at one in the morning — a curly quote pasted from a web page, a tab where a
space was intended, Windows line endings from an editor on another machine, a
`#` inside a quoted string.

A hand-written parser gets those cases wrong quietly. What the user sees is
shanty reporting that a credential is missing while the credential is plainly
there on screen. That is a bad hour for someone who did nothing wrong, and it
is a class of bug this project has no cheap test for.

Cost: one direct dependency and one module, against budgets of 10 and 30.

**Refused alongside it:** a second configuration format. No JSON, no YAML, and
no per-key environment variable overrides. Two files in one format is the
entire surface. "shanty is ignoring my configuration", caused by an overriding
mechanism the user forgot was enabled, is exactly the failure a single format
avoids.

---

## ADR-011 — Playback uses mpv, in a separate process

**Status:** accepted. The consequences of this were written throughout the plan
from the start, but the choice itself was never argued. This record does that.

The alternative is a Go audio library: `oto` for output with decoders alongside
it, or `beep` built on top. It wins on the two things this project claims to
care about most. There would be no prerequisite to install at all, and decoding
would happen inside the process where the fast test suite could exercise it,
rather than behind a process boundary that suite cannot cross.

Both advantages are real. Neither survives the three arguments against.

**It does not fit the module budget**, by a wide margin rather than a narrow
one. Measured with the interface libraries and test helpers included on both
sides: mpv comes to 28 modules against a budget of 30; `oto` with a single MP3
decoder comes to 33; adding FLAC brings it to 43; `beep` brings it to 55.

No version of the Go path fits, and the smallest one supports only MP3, which
means every track is transcoded by the server. That spends the server's
processor time converting a lossless library into a lossy stream, which is the
wrong trade for someone who chose to run their own music server.

**It contradicts ADR-001.** That record answers "what can a compromised server
do" partly by putting stream data in mpv's hands. A Go decoder moves the
parsing of untrusted binary audio into the process holding the credential. mpv
is not chosen because it cannot be exploited; it is chosen because it is a
separate process. A malformed FLAC there is a player shanty reports as stopped,
rather than a corrupted process holding a token.

**It is a large amount of code that would have to be correct.** Seeking within
an HTTP stream, opening the next track early so albums play without gaps,
resampling, output device selection, and volume normalisation are all provided
by mpv and would all have to be written and got right, against a 6,000-line
limit.

The costs of this decision are accepted and worth naming. mpv is an external
runtime dependency that the module budget cannot see, so the largest thing
shanty depends on is the one thing no number here counts. Playback cannot be
tested by anything in the fast suite, which is why the integration test is not
optional. And `doctor` has to know the install command for each platform,
because "install mpv" on its own is not a usable instruction.

This is reversible. `internal/mpv` is a few hundred lines behind a narrow
interface, so if the prerequisite proves more costly than expected, replacing
it is a contained change rather than a rewrite.

---

## ADR-012 — macOS support is not yet described

**Status:** open. Recorded before it is needed.

Everything decided so far is settled for Linux and applies to macOS in
intent — one mpv process, one socket, the credential passed over that socket
and never in command-line arguments. What is not settled is the specific shape
that takes on macOS, and the code already differs there in ways the plan does
not acknowledge.

Checked against the Go standard library rather than assumed:
`os.UserConfigDir` returns `~/Library/Application Support` on macOS, and
`os.UserCacheDir` returns `~/Library/Caches`. The filesystem contract of four
directories written as `~/.config/shanty` and so on therefore describes Linux
only. `Discover` already resolves different paths on macOS, and the isolation
tests will assert a different set of written files depending on the platform.

The open questions, to be answered together rather than one at a time as each
first causes a problem:

- Which four directories are the contract on macOS? Does the plan gain a second
  table, or does shanty use XDG paths everywhere regardless of platform
  convention?
- macOS has no `XDG_RUNTIME_DIR`. The socket falls back to the cache directory,
  which unlike a runtime directory is not cleared at logout, so a socket left
  behind survives a reboot. shanty removes a stale one at startup, but a socket
  living in a cache is a different promise from one that disappears.
- The process hygiene test reads `/proc`, which does not exist on macOS. The
  child process's own report of its arguments covers macOS today. Whether the
  kernel's view should also be checked, through `sysctl KERN_PROCARGS2`, is
  undecided.
- shanty passes its whole environment to mpv because PipeWire, PulseAudio and
  ALSA read from it. CoreAudio does not, so the reasoning that justifies
  inheriting the environment does not apply on macOS and the decision should be
  made again rather than assumed.
- A downloaded binary is quarantined by macOS until it is signed or notarised.
  This has to be answered before a release claims to support macOS.

Recorded now because these are architectural questions rather than packaging
ones. Answering them after the Linux implementation has settled is how a
project ends up with a second, worse implementation on the platform nobody
develops on.

---

## ADR-013 — Text from the server is stripped of terminal control sequences before it is displayed

**Status:** accepted. Amends ADR-001, which stands. This adds a consequence
that record was missing.

ADR-001 asks what a compromised server can do and answers in three parts: it
controls the JSON parsed, the image data decoded, and the strings that might
become filesystem paths. It missed a fourth, which is the one this program
reaches first.

Every artist name, album title and track title is chosen by the server and
printed to a terminal. A terminal is not a display; it is an interpreter. The
sequences it acts on include moving the cursor, clearing the screen, changing
the scrolling region, and on many terminal emulators a query whose reply is
inserted into the input stream as though the user had typed it. A track title
followed by an escape sequence can redraw the screen around itself, or send
characters to whatever is reading the keyboard.

This requires no vulnerability in any parser. Every field the browsing screens
display can carry it.

**Therefore no string from the server reaches the terminal without passing
through one function that removes those sequences, and that function is called
at the point of rendering, in `internal/tui`.**

Two placements were considered. Removing the sequences when the response is
decoded, in `internal/subsonic`, would be a stronger guarantee, because nothing
downstream could ever hold an unsafe string. It was rejected because it changes
what the server sent. A track genuinely containing an unusual character would
come back altered, and the play report sent to the server would carry the
altered form, so shanty would be describing a track the server does not have.
Rendering is the point at which a string stops being data and becomes an
instruction, so that is where it is stopped.

**What is removed:** the C0 control characters, DEL, and the C1 range, which
covers every byte a terminal can read as the start of a sequence. Newline and
tab are removed with them, because a title occupies one line and one containing
a newline breaks the layout whether or not that was intended.

**What is not removed:** the Unicode bidirectional override characters. They
can reorder text visually, which is a genuine attack in source code, but an
artist name in Arabic or Hebrew is an ordinary thing for this program to
display. Removing them would break real names in order to prevent a display
trick that affects nobody. If that assessment changes, it becomes a later
record rather than a quiet edit to this one.

The fake Subsonic server gained a mode that returns hostile titles, so this is
checked by every screen's rendering test rather than by one function's unit
test.

---

## ADR-014 — mpv must run in the same filesystem namespace as shanty

**Status:** accepted. Amends ADR-011, which decided that playback happens in a
separate process without saying where that process has to be.

The socket is the reason for ADR-011: the credential travels over it so that it
never appears in command-line arguments. A unix socket is a file, so shanty and
mpv have to see the same filesystem at the same path. That was an unstated
assumption and it is load-bearing.

What it rules out:

- **mpv installed as a flatpak.** The sandbox gives the application its own
  `XDG_RUNTIME_DIR`, so the socket shanty creates is not present under the path
  shanty told mpv to open. The failure looks like a permissions problem.
- **mpv in a container shanty is not running in**, and the reverse. On a host
  managed by rpm-ostree with a development container alongside it, that is the
  normal arrangement rather than an unusual one, and both programs have to be on
  the same side of it.

What it requires depends on the machine: a package manager that writes to the
filesystem shanty runs from. On an ordinary distribution that is the usual
install command. On an rpm-ostree host it is `rpm-ostree install mpv` followed
by a reboot. Inside a development container it is that container's own package
manager, and shanty has to run in the same container.

`doctor` reports which of these applies, because naming a command the machine
cannot run is worse than naming none. Telling a user of an image-based system
to run `dnf install mpv` produces an error they then have to diagnose.

The alternative was to let mpv run anywhere and pass the stream URL some other
way, which means command-line arguments, which ADR-001 refuses. The constraint
is the price of that property.

---

## Open, and assigned

Decisions still to be made. Each becomes a record above once it is settled,
including when it is settled the way the plan expects.

| # | Question |
|---|---|
| ADR-006 | Whether to support MPRIS, and whether D-Bus fits the module budget. Belongs to v0.3. |
| ADR-010 | The measured module cost of the interface libraries chosen in ADR-003. |
| ADR-012 | The macOS form of the filesystem and process decisions, described above. To be answered before a release claims macOS support. |
