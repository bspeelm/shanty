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

**Status:** open, and one question below is now answered. Recorded before it
was needed.

**Answered: shanty uses XDG paths on every platform, macOS included.** All four
directories are resolved from the XDG variables by shanty rather than by
`os.UserConfigDir` and `os.UserCacheDir`, which return `~/Library` paths on
macOS and ignore the environment variables entirely.

The deciding argument is fish. The fish completion goes beside fish's own
configuration, which is `~/.config/fish` on macOS as much as on Linux, because
fish follows XDG on both. Resolving shanty's configuration directory the Apple
way put that file in `~/Library/Application Support/fish/completions`, where
fish does not look. A macOS-native table would have had to except fish from
itself, which is the shape of a rule that is not the right rule.

The rest follows from that: one filesystem contract, one write set for the
isolation suite to hold, one set of paths in §8 and in the wiki, and
`XDG_CONFIG_HOME` working the same way in both places. The cost is that shanty
does not follow the platform convention on macOS, which is a cost paid by a
program whose configuration is a text file people are expected to edit.

The questions below this line are still open.

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
  The Homebrew cask strips the attribute on install, which makes that route
  work. A tarball downloaded by hand still does not run, and signing properly
  is unanswered.

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

## ADR-015 — Bidirectional formatting characters are removed from server text

**Status:** accepted. Amends ADR-013, which stands.

ADR-013 decided that terminal control sequences are removed from text the
server supplies, and decided not to remove the Unicode bidirectional
characters. The reason it gave was that an artist name in Arabic or Hebrew is
an ordinary thing to display, and that removing those characters would break
real names to prevent a display trick affecting nobody.

That reasoning rests on a false premise. Arabic and Hebrew render correctly
from the directional properties of the letters themselves, through the Unicode
Bidirectional Algorithm. They need no control characters. Removing the control
characters breaks no name written in a right-to-left script.

The test written to defend the earlier position shows the confusion. Its input
was Arabic letters wrapped in `U+202B RIGHT-TO-LEFT EMBEDDING` and `U+202C POP
DIRECTIONAL FORMATTING`. The letters survive stripping either way. What the
test actually asserted was that the two control characters survive.

**What these characters do.** An override or an isolate forces the direction of
strong characters, so a server can make a title display in an order the
characters do not have. A track named to end in one thing can be made to read
as ending in another. The screens are also built by joining a title and a
duration into one line, so an unterminated directional control inside a title
changes the layout of the rest of that row.

**Therefore the bidirectional formatting characters are removed alongside the
terminal controls:** `U+202A` to `U+202E`, the embedding and override block
that Unicode itself deprecates, and `U+2066` to `U+2069`, the isolates.

**The marks `U+200E` and `U+200F` are kept.** They act as invisible strong
characters that resolve the direction of neutral characters between them. They
cannot reverse a run of letters, so they cannot produce the effect above, and
they are what a person uses to place punctuation correctly in a title that
mixes directions.

The fake server's hostile-text mode carries a direction override, so this is
checked by every screen's rendering test rather than by one function's unit
test.

## ADR-016 — Key bindings are ordered by consequence, with three modes and a command line

**Status:** accepted. The bindings are implemented against this rather than
chosen per feature.

shanty has sixteen bindings for three screens. v0.2 adds filtering, server
search, play next, append, a queue screen, a playlists screen, adding to and
removing from playlists, creating and deleting them, starring, a starred view
and seeking to a position: about thirty bindings in total. Assigned one feature
at a time, each takes whatever key was free that week, and the result is a set
nobody could have designed on purpose.

Configurable keys also cannot be designed without this. A binding
configuration binds to a named set of actions, and that set is what this record
fixes.

### The ordering principle

**Keys are ordered by consequence, not by frequency alone.**

Frequency puts the most-used actions on the easiest keys, which is correct as
far as it goes and says nothing about what happens when the wrong key is
pressed. Ordering by consequence answers both questions at once: the keys that
are easiest to hit are the ones where hitting them by mistake costs nothing.

- Bare keys do things that are reversible and local. Nothing on a bare key
  touches the server or ends the session.
- Going to a screen is prefixed with `g`, which already means "go".
- Changing something on the server that shanty cannot undo asks first.
- Everything rare is a command, so it needs no key at all.

### Three modes

**Normal** is the default and is where every binding below applies.

**Filter**, entered with `/`, narrows the list on screen as characters are
typed. `esc` cancels and restores the list; `enter` keeps the filter and
returns to normal mode.

**Command**, entered with `:`, takes a line and runs it. `esc` cancels; `enter`
runs.

Both text modes leave with `esc`, and neither is entered by accident.

### Normal mode

Movement, all reversible:

| key | action |
|---|---|
| `k` `↑` | up |
| `j` `↓` | down |
| `gg` | first row |
| `G` | last row |
| `ctrl+d` `ctrl+u` | half a screen down, up |
| `{count}` before a movement | repeat it |

Navigation:

| key | action |
|---|---|
| `enter` `l` `→` | open, or play when the row is a track |
| `esc` `h` `←` | back one screen |

Going to a screen:

| key | action |
|---|---|
| `ga` | artists |
| `gq` | queue |
| `gp` | playlists |
| `gs` | starred |

Transport, all reversible and all local:

| key | action |
|---|---|
| `space` | pause or resume |
| `n` `p` | next, previous track |
| `]` `[` | seek forward, back |
| `+` `-` | volume |
| `{count}%` | seek to that percentage of the track |

Acting on what is selected:

| key | action |
|---|---|
| `s` | star or unstar; pressing it again undoes it, so it needs no confirmation |
| `a` | add to a playlist; additive and undone by removing |
| `dd` | remove the selected track from the playlist being viewed; asks first |

### Commands

`:q` quits. `:help`. `:search <query>` searches the server. `:playlist new
<name>`, `:playlist rename`, `:playlist delete` — the last asks first.
`:volume 50`. `:seek 1:23`.

The long tail lives here, so a rare action never has to claim a letter.

**Amended:** there is no `:help`. The command that explains the commands is
`:wiki`, which is the name the wiki already had and the one somebody looking
for the documentation would reach for. It shows the list, and a page for each
saying what it does and how it is typed.

`:help` is not bound to it. Two names for one command would break the closed
set the command table is held to, and a set that admits synonyms stops being a
list of what exists.

### Quitting

**`:q`.** Quitting ends the session and cannot be undone from inside shanty, so
it is not a bare key.

`ctrl+c` continues to quit, because it is what people try when a program will
not let go, and it works at a level shanty does not control.

`q` is bound to printing `type :q to quit` on the status line. Leaving it
unbound would be silent, and someone who has learned `q` elsewhere deserves to
be told where it went rather than pressing it repeatedly.

### What was taken from vim and what was not

Modes, a command line, `g` meaning go, `gg` and `G`, counts, and `{count}%`
meaning a percentage of the whole all transfer directly, and mean here what
they mean there.

Operators applied to motions do not transfer. They work in an editor because
text has ranges to act on. A list of tracks has rows, and inventing objects so
that `d3j` means something would be imitating the shape of the idea without the
thing that made it work. `dd` is kept as a single binding that reads like the
editor's, not as an operator with a motion.

### Consequences

Digits are counts, so seeking to a position is `{count}%` rather than a digit
on its own. That is the same meaning digits have in the editor, and it is why
the progress bar issue does not get `0` to `9`.

The action names in the tables above are the set that configurable keys binds
to. A binding added later without a name here is a binding nothing can
configure and nothing documents.

A test holds the bound actions against the documented ones in both directions,
in the same way the command list is already held against the README.

**Amended:** that test exists. Keys dispatch through a table of named actions
rather than a switch, and `TestEveryKeyIsDocumentedAndEveryDocumentedKeyIsBound`
holds the table against `wiki/Keys.md` both ways. The keys the page explains in
prose, and the things it writes in the same style that are not keys, are each
named in the test with the reason.

## ADR-017 — The command line shows what exists, and holds only what keys cannot

**Status:** accepted. Extends ADR-016, which established that `:` exists for
the long tail without saying what it contains or how it is entered.

### The argument against having one at all

`:` is a text-entry surface in a program driven by arrow keys and a space bar.
A user may type a command no times in a session. Quitting is the only thing
that truly needs it, and a single key would do that. It costs line editing,
parsing, error reporting and somewhere to list what exists. Worst of all it
invites scope: once a command line exists, a feature request becomes "add a
command for it" rather than being refused, and the non-goals are the fence that
keeps this program small.

### Why it survives

**The text-entry widget is being built regardless.** Filtering needs it.
Naming a new playlist needs it. Searching the server needs it. Given that, `:`
is a second use of something already written, plus a command word and a table.

**And the commands are things keys cannot express.** A command takes an
argument or is rare. Nothing that could be a key becomes a command instead, so
`:` is not a second way to do what the keyboard already does. This is the
distinction that keeps it from being a duplicate interface, and it is also the
rule that decides membership.

### Entering a command shows what exists

`:` opens a line. Typing filters a list of matching command names, shown above
it. Tab completes the longest common prefix.

The list is the reason to prefer this over a bare prompt. A blank line tells a
new user nothing and requires documentation to be useful; a list that appears
as soon as they press `:` makes the command set learnable from inside the
program. It is also why there is no separate help command listing them: `:` on
its own already shows everything.

### What is in it

Taking an argument, so they cannot be keys:

`:playlist new <name>`, `:playlist rename <name>`, `:playlist delete`,
`:search <query>`, `:seek 1:23`, `:volume 50`, `:server <name>`.

Rare enough not to deserve a key:

`:q`, `:version`, `:reload`, `:scan`, `:messages`.

`:seek` and `:volume` take an absolute value where the keys are relative, so
they are not duplicates of `[`, `]`, `+` and `-`.

### The rule, restated because it is the whole point

A command exists because it takes an argument, or because it is rare. Something
that is neither gets a key, or does not exist. `:` is not a route around the
non-goals, and a request that does not fit a key does not automatically fit
here.

### Consequences

Keys bind to named actions and commands are a separate, smaller table. These
are two lists rather than one, and each is held against its documentation by a
test.

`:reload`, `:scan` and `:messages` name things shanty cannot currently do, and
are tracked as their own work rather than as part of building the command line.

---

## Open, and assigned

Decisions still to be made. Each becomes a record above once it is settled,
including when it is settled the way the plan expects.

| # | Question |
|---|---|
| ADR-006 | Whether to support MPRIS, and whether D-Bus fits the module budget. Belongs to v0.3. |
| ADR-010 | The measured module cost of the interface libraries chosen in ADR-003. |
| ADR-012 | The macOS form of the filesystem and process decisions, described above. To be answered before a release claims macOS support. |

## ADR-018 — A session that outlives its terminal is not a daemon

**Status:** accepted. Amends PLAN.md §3, which lists a daemon among the things
shanty is not.

`:headless` closes the interface and leaves the music playing. Seven commands
drive what is left from any shell, and running `shanty` again returns the
interface to it.

### What §3 refuses, and why this is not it

§3's not-list is binding, and "A daemon." is on it. The distinction this record
rests on is that a session is created only by a keystroke in a running
interface. It is never started at login, never by a service manager, never
respawned after it dies, and it ends by itself when its queue runs out. It
exists because somebody asked for their terminal back, and it lasts as long as
the music they were already playing.

A process that outlives the user's intent is what §3 refuses. This one cannot
outlive it by more than an album.

### Why a session, rather than leaving mpv to it

mpv plays a playlist to the end on its own, and a second process can drive its
socket. So the cheaper design was to hand mpv the album and exit shanty
completely. It was rejected twice over.

**It breaks the fifth verb.** `docs/north-star.md` states the invariant as
connect, browse, queue, play, tell the server you played it. The report is sent
by the running shanty process when mpv says a track ended. With no shanty
process, an album plays and the server never hears about it: no play counts,
nothing in recently-played. The cheap design deletes a fifth of what the
program is for, in the mode where the user is least likely to notice.

**It would make mpv's socket the user-facing control plane.** mpv's IPC accepts
`run`, `subprocess` and `load-script`. Anything that can write to that socket
can start a process. `shanty pause` must not be a wrapper around a channel like
that, so shanty keeps mpv's socket to itself and answers on one of its own,
where the verbs are a closed set and none of them names a file or a program.

### The control plane

Newline-delimited JSON, one request and one response per connection, on a
socket in the same 0700 directory as mpv's and narrowed to 0600. The verbs are
the same named actions the keys and the command line use, so a command typed in
another shell does exactly what the key would have done.

Every request carries the version of the wire format. A session refuses a
version it does not speak and names `shanty stop`. **`stop` and that refusal
cannot change meaning in any later version**, because they are the only two
things a mismatched pair still has to be able to do. That is a commitment this
record makes, not an implementation detail.

### Who owns the player

Exactly one process drives mpv at a time, and the control socket says which.
Two would both see mpv's events and both advance the queue, reporting one track
twice and skipping the next.

So every handover is the same shape in both directions: the newcomer attaches
to the player, and the incumbent lets go without stopping it. The process
handing over stops advancing the queue the moment the handover starts rather
than when it succeeds. If it fails, one report is lost; if it were the other
way round, one would be made twice on the path that works.

### What a session cannot know

A session holds the credential it was started with. Revoking an API key does
not stop one that is already playing; it plays out its queue with a credential
the server will refuse, and the failed reports are not shown to anybody.

This is not detectable without polling the server, which §9 refuses. It is
instead **bounded by the session ending when its queue runs out**, which is the
security argument for the framing rather than merely an aesthetic one. A daemon
would hold a revoked credential indefinitely. `shanty doctor` says when a
session is running, so the question has an answer somebody can type.

### The costs, accepted

A process that plays where nobody is looking cannot report its own failures,
and `docs/north-star.md` claims shanty is honest when broken. `doctor` gains a
check naming all four states a session and a player can be in between them.
`uninstall` refuses while a session is playing, because it removes the
directory holding the socket the session is reached through.

The interface cannot detach in place: a program started from a shell leads its
process group, so it cannot leave its session. It starts a copy of itself
instead, which is the first time shanty has run anything but mpv, and brings
that copy under the same rule — no credential in a child's arguments or
environment. The queue reaches it by id over a pipe, and the copy builds its
own stream URLs from the credential it reads itself.

## ADR-019 — What a key means can follow the mode, and `a` does

**Status:** accepted. Amends ADR-016, which gave `a` to playlists.

ADR-016's table reads `a` — add to a playlist. It was built as `a` — add to the
end of the queue, with `A` for play next, and the difference was not noticed
until playlists were built.

Both are right in their place, and the place is what decides.

Outside `:playlist edit`, the queue is what a track is added to: it is the
thing playing, and adding to it is the commonest action in the program. Inside
`:playlist edit` the library lists exist for one purpose, which is to say what
belongs in the playlist. `a` adds and `r` removes, and there is nothing else
those keys could sensibly do while that screen is up.

### Why a key may mean two things

The rule this rests on is that a mode is visible. `:playlist edit` says what it
is doing on the last row, every list carries `(A)` beside the tracks already in
the playlist, and `e` leaves. Somebody pressing `a` can see which of the two it
will do before they press it.

**Amended: the key that leaves is `e`, and `esc` moves up a screen.** As first
built, `esc` left the playlist rather than going up, on the reasoning that
leaving the mode mattered more than leaving the screen. That reasoning had the
mode backwards. Edit mode shows the library because tracks have to be found
anywhere in it, and reaching a second album means going back up to the album
list. Giving `esc` to the mode meant edit mode could reach exactly one album
before the whole command had to be typed again.

So `esc` keeps the one meaning it has everywhere, `e` leaves, and edit mode
lasts however far the library is browsed. The visibility rule costs more under
this arrangement and is paid: the artist and album lists carry no track marks,
so the last row names the playlist for as long as the mode is on rather than
until the next message replaces it.

That is the same test the filter and the command line already pass: `/` and `:`
change what typing does, and both show it. A mode that could not be seen would
not be allowed to change a key.

### What was refused

**Giving `a` back to playlists and moving the queue keys.** The queue keys are
already in use and already documented; rebinding them to honour a table written
before either feature existed would be paying for consistency with a plan
rather than with the program.

**Two more letters, so nothing is modal.** The alphabet is the scarce thing
here. ADR-016 spends it on the actions used most, and adding to a playlist,
which happens in one screen and is otherwise rare, is not one of them.

### The confirmation

`:playlist delete <name>` asks before it acts and names the playlist and how
many tracks are in it. Typing the name is most of a confirmation, but a name
that matches something other than what was meant is exactly the mistake that
cannot be undone from inside shanty. Every key but `y` answers no.

A server allows two playlists to share a name; only their identifiers are
unique. Two matches is reported rather than resolved, because picking one would
be a guess about the thing that cannot be undone.

## ADR-020 — Keys are bound to named actions, and the names are the interface

**Status:** accepted. Completes ADR-016, which planned configurable bindings
without saying what a binding names.

A key does not do a thing; it names one. `next` is the action, and `n` is one
way to ask for it. The table of actions is what the configuration binds
against, what the wiki is held to, and what `doctor` lists.

This is why the dispatch is a table rather than a switch. A switch is not a
list of anything: it cannot be counted, compared with a page, or rebound. The
table is a few lines larger and it is the only shape in which ADR-016's
promised test could be written at all.

### What the configuration may change, and what it may not

An action named in `[keys]` takes the keys it is given instead of the ones it
came with. An action left out keeps them, so a configuration says only what is
different.

The keys that open a mode -- `/`, `:` and `g` -- are ordinary actions and can
be rebound. The keys inside a mode cannot: `esc` leaves a filter, `tab`
completes a command, `y` answers a question. They belong to the mode rather
than to the list, and a mode with a rebindable escape is a mode somebody can
lock themselves inside.

### Everything wrong is reported, and nothing refuses to start

A binding onto a key another action still holds leaves one of them silently
unreachable, which is the commonest way to get this wrong and the hardest to
notice. It is reported, along with an action that does not exist and one bound
to no keys at all.

All of them at once, rather than the first: somebody fixing a configuration
wants the list. `doctor` gives it, and shanty says it on the last row when it
starts.

It starts anyway. A typo in `config.toml` is not a reason to withhold somebody's
music, and the alternative -- refusing to run until the file is right -- makes
the mistake worse than it is.

