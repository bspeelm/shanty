# Decisions

Numbered, append-only. A record is superseded or amended by a later record,
never edited away and never deleted. Titles are sentences that state the
decision, not topics — "The core is Go", not "Language choice".

**Refusals are recorded here too**, and they are the half that earns this file
its keep: a decision already declined is settled, and a re-proposal is answered
with its number rather than a fresh argument.

---

## ADR-001 — What a stolen credential, a hostile server, and a nosy neighbour each get

**Status:** accepted, before any code. PLAN.md §2 is the short form; this is
the record.

Four questions, asked in this order because each one's answer constrains the
next. Every "therefore" below is a test named in PLAN.md §5, and the point of
writing them now is that the audit which produced this project found five
shipping clients that had answered them silently, and wrongly.

**What does a stolen config yield?** It depends entirely on what is in it, so
shanty prefers credential modes in one order and the order is the decision:

1. An **OpenSubsonic API key** — one revocable string, killed server-side in a
   click. Navidrome supports these.
2. A **hashed token and salt** — replayable against this server and useless
   anywhere else, because it is not the password.
3. A **password file** the user manages — an agenix secret, a `pass(1)` entry,
   whatever they already trust.

A plaintext password is the fourth thing and is never written by shanty. It is
*read* if the user wrote one by hand, because refusing outright pushes people
toward the worst client that will accept it, and `doctor` names the better
option every time it sees one. Declining to write it is a promise shanty can
keep; declining to read it only moves the problem.

**What can another local user see?** Process argv, the environment of anything
we spawn, world-readable files, and a shared `/tmp`. Therefore: no token
reaches a child's argv or environment, credentials live 0600 in a file of their
own, and the mpv socket lives under `XDG_RUNTIME_DIR` in a 0700 directory. The
audit found a client passing token-bearing stream URLs in argv, visible to
every account on the machine through `ps`.

**What can a malicious or compromised server do?** It controls every byte we
parse — JSON, cover art, lyrics, stream data. Therefore response parsing is
fuzzed, image data is size-limited before it reaches a renderer, response
bodies are read through a capped `io.LimitReader`, and a server-supplied string
never becomes a filesystem path except through one sanitising function that has
its own tests. The threat is not that the server is evil; it is that "my
server" and "a server" are the same code path.

**What can the network do?** Delay forever, or lie. Every HTTP client carries a
timeout. **TLS verification has no off switch** — not a flag, not a config key,
not an environment variable. A user with a self-signed certificate adds it to
their trust store, which is the mechanism that already exists for exactly this.
An `insecure = true` knob is one paste away from living in every setup guide on
the internet, and a knob that exists gets turned. `make budgets` greps for
`InsecureSkipVerify` so this cannot arrive by accident.

The cost of the last one is real and is accepted: someone with a self-signed
cert and no interest in their trust store will find shanty harder to use than
a client that offers the flag. That is the intended trade.

---

## ADR-002 — The name is shanty

**Status:** accepted.

A shanty is a rough hut and a song sung by people working, which is the whole
program in one word: something small you built yourself, and music that comes
out of it. No conflicting Go module, no conflicting binary on a normal PATH.

The decision is recorded now rather than left pleasant and open because the name
is not only a label. It is the module path, the binary, all four directories in
§8, and the `c=` client identifier every Subsonic request carries — which
servers log, and which users filter their play history by. Renaming after a
release moves someone's config directory out from under them and orphans their
scrobble history on the server. The cost of choosing is one afternoon of taste;
the cost of deferring is paid by anyone who installed early.

## ADR-003 — The TUI is Bubble Tea, and the module budget is the ceiling it fits under

**Status:** accepted.

Bubble Tea is the best-tested path in this domain, and `teatest` gives
golden-file rendering tests without building a harness first. Since every screen
in §12 is meant to be a readable diff when it regresses, a framework that hands
us that for free is worth more here than in most projects.

The counter-argument, which is real: the charm stack is the largest single
dependency decision this project will make, and §0 budgets 30 modules total.
Bubble Tea with Bubbles, Lipgloss and `teatest` is estimated at 20–24 of them,
leaving very little room for anything else, forever.

The decision resolves that tension in the budget's favour rather than the
framework's. **Bubble Tea is adopted; 30 is a hard ceiling, not an opening
offer.** The count is measured before the dependency is committed, and if it
does not fit, Bubbles and Lipgloss are dropped and the rendering is hand-rolled
against Bubble Tea's core. The budget is not renegotiated to admit a
convenience. ADR-010 records the measured number.

## ADR-004 — TLS verification has no off switch

**Status:** accepted. A standing refusal.

Not a flag, not a config key, not an environment variable, not a build tag.
ADR-001 argues it; this record exists so the refusal has a number to be answered
with, because it will be asked for.

A user with a self-signed certificate adds it to their trust store, which is the
mechanism that already exists for exactly this problem and which every other
program on their machine already respects. An `insecure = true` knob is one
paste away from living in every setup guide on the internet, and a knob that
exists gets turned — by people who do not know what they are turning off, in
setups they did not write.

The cost is accepted and is not small: someone with a self-signed cert and no
interest in their trust store will find shanty harder to use than a client that
offers the flag. `make budgets` greps for `InsecureSkipVerify` so this cannot
arrive by accident, and `doctor` should name the trust-store fix when a
certificate error is what it sees.

## ADR-005 — Scrobbling goes to the server and nowhere else

**Status:** accepted. A standing refusal.

shanty calls the Subsonic `scrobble` endpoint. It does not talk to Last.fm,
ListenBrainz, Libre.fm, or anything else, and it never will through this
document.

The reasoning is a trust boundary, not a scope preference. Navidrome already
federates outward when its administrator configures it to, and the administrator
is the same person running this client. Adding direct scrobbling would mean a
second credential store, on every machine the client is installed on, for a
service the server can already reach — multiplying the number of places a
secret lives in order to save one configuration step in a place the user already
administers. §9's one-host rule falls out of this: shanty talks to exactly one
host, so "what does this program contact" has the same answer as "what did I
configure".

"Add Last.fm" is expected to be the first feature request. It is answered with
this number.

## ADR-007 — Navidrome current-minus-two is the floor, and every other server is best-effort

**Status:** accepted.

shanty declares Subsonic API version `1.16.1` and targets Navidrome from the
current release back two minor versions. OpenSubsonic extensions — API keys
chief among them — are **detected, never assumed**: the client asks what the
server supports and degrades to token+salt when the answer is no.

Other Subsonic-compatible servers are welcome to work and are not tested. That
is the honest half of this record: the integration job in §11 runs one server
image, so Navidrome is the only compatibility claim that has a proof behind it.
Claiming gonic, Airsonic and Ampache support without a job that exercises them
would be marketing, and every user-facing claim in this project names the test
that proves it or it goes. A second server image can be added later, and the
claim arrives with it — in that order.

## ADR-008 — The licence is MIT

**Status:** accepted.

MIT, matching bothy, so one maintainer carries one licence across both projects
and a reader of either does not have to check. Nothing in shanty is derived from
a copyleft source, so there is no obligation pulling the other way, and the
permissive choice keeps the door open for anyone who wants to lift the fake
server or the isolation suite into their own client — which, given what the
audit found in this field, would be a good outcome.

## ADR-009 — Configuration is TOML, read by pelletier/go-toml/v2

**Status:** accepted, before the dependency is added.

The strongest argument against is that this buys almost nothing. Both files are
a handful of `key = "value"` lines; sixty lines of `strings.Cut` over a scanner
would read them, cost no budget slot, and never need a security advisory read.
For parsing what shanty itself writes, that is entirely true.

It is not what the parser is for. §6 makes both files hand-edited on purpose —
a `password_file` pointing at an agenix secret or a `pass(1)` entry is
something a person types, and the parser's real job is reading what somebody
wrote at one in the morning: a smart quote pasted from a web page, a tab where
a space was meant, CRLF from an editor on another machine, a `#` inside a
quoted string. A hand-rolled parser gets those wrong quietly, and the failure a
user sees is "shanty says my credential is missing" while the credential is
plainly there on the screen. That is a bad hour for someone who did nothing
wrong, and it is the kind of bug this project has no cheap test for.

The second reason is smaller and still real: this is bothy's only dependency,
at the same version. One maintainer reads one advisory when one arrives.

Cost: one direct dependency and one module, against budgets of 10 and 30.

**Refused with it:** a second configuration format. No JSON, no YAML, no
per-key environment overrides. Two files in one format is the whole surface,
and "shanty ignores my config" caused by a shadowing mechanism nobody
remembered enabling is precisely the failure the single format avoids.

## ADR-011 — Playback is mpv, in a process of its own

**Status:** accepted. The consequences were written throughout PLAN.md §7 from
the start; the choice itself was never argued, which is what this record fixes.

The alternative is a Go audio library — `oto` for output with decoders beside
it, or `beep` over the top — and it wins on the two things this project says it
cares about most. There would be **no prerequisite at all**: `go install` and
the user is listening, against a north star that measures itself in "under two
minutes". And decoding would happen in-process, where `make check` could test
it, instead of behind a process boundary nothing in the fast suite can cross.

Both are real. Neither survives the other three.

**The budget says no**, and by a margin rather than a hair. Measured, with the
TUI and `teatest` on both sides: mpv is 28 modules against a budget of 30; one
mp3 decoder and `oto` is 33; adding FLAC is 43; `beep` is 55. There is no
version of the Go path that fits, and the smallest one buys only mp3 — which
means every play is transcoded server-side, spending the server's CPU to turn
the user's lossless library into a lossy stream. That is the wrong trade for
someone who runs Navidrome on purpose.

**It contradicts ADR-001.** That record answers "what can a malicious or
compromised server do" with, in part, *stream data goes to mpv, whose job that
is*. A Go decoder moves the parsing of hostile binary audio into the process
holding the credential. mpv is not chosen for being unexploitable; it is chosen
for being **elsewhere** — a malformed FLAC there is a player we report as dead,
not a corruption in the program with the token in memory. Choosing otherwise
would not be a new decision, it would be an amendment weakening an existing
one.

**And it is a great deal of code we would have to be right about.** Seeking
inside an HTTP stream, gapless prefetch, resampling, output device selection,
ReplayGain: all free from mpv, all ours to write and to get wrong audibly,
against a 6,000-line cap.

The costs are accepted and named, because they are real. mpv is an external
runtime dependency that §0's budget cannot see, so the largest thing shanty
depends on is the one thing no number here counts. Playback cannot be tested by
anything in `make check`, which is the whole reason the integration job in §11
is not optional. And `doctor` must name the install command per platform, since
"install mpv" is not a fix line.

This is not a one-way door: `internal/mpv` is a couple of hundred lines behind
a narrow interface, so if the prerequisite turns out to cost more than it looks
like, replacing it is contained rather than a rewrite.

## ADR-012 — macOS gets the same shape, and the shape is not yet described

**Status:** open, and deliberately recorded before it is needed.

Everything above is settled for Linux and applies unchanged to macOS in
intent — one mpv, one socket, the credential over IPC and never in argv. What
is **not** settled is the shape that intent takes there, and the code already
diverges in ways PLAN.md §8 does not admit to.

Verified against the Go standard library rather than assumed:
`os.UserConfigDir` returns `~/Library/Application Support` on darwin, and
`os.UserCacheDir` returns `~/Library/Caches`. So §8's filesystem contract —
four directories, written as `~/.config/shanty` and the rest — describes Linux
only. `Discover` today resolves somewhere else entirely on macOS, and the
isolation suite will assert a write set that differs by platform.

The open questions, to be answered together rather than one at a time when each
first bites:

- Which four directories are the contract on macOS, and does §8 gain a second
  table or does shanty use XDG paths everywhere regardless of platform custom?
- There is no `XDG_RUNTIME_DIR`. The socket falls back to the cache directory,
  which unlike a runtime directory is **not** cleared at logout, so a stale
  socket outlives a reboot. `Start` already removes one, but a socket living in
  a cache is a different promise from one that disappears.
- `TestChildProcessHygiene` reads `/proc`, which does not exist there. The
  child's own report covers macOS today; whether the kernel's view should be
  checked through `sysctl KERN_PROCARGS2` is undecided.
- `Start` inherits the environment because PipeWire, PulseAudio and ALSA need
  it. CoreAudio does not, so the reasoning that justifies inheriting is
  Linux-only, and the decision should be re-argued rather than inherited.
- A downloaded binary is quarantined until signed or notarised, and the sibling
  project has already met this one: *macOS refuses the binary, and nothing said
  why*. §11 must answer it before the first release claims darwin support.

Recorded now because these are architecture, not packaging. Answering them
after the Linux shape has hardened is how a client ends up with a second,
worse implementation on the platform nobody developed on.

## ADR-013 — The terminal is a fourth thing a hostile server can reach, and ADR-001 did not say so

**Status:** accepted. Amends ADR-001, which stands; this adds a therefore it
was missing.

ADR-001 asks what a malicious or compromised server can do and answers in three
parts: it controls the JSON we parse, the image data we decode, and the strings
we might turn into filesystem paths. It missed the fourth, which is the one
v0.1 actually reaches first.

Every artist name, album title and track title is chosen by the server and
printed to a terminal. A terminal is not a text display; it is an interpreter,
and the bytes it interprets include cursor movement, screen clearing, scroll
region changes, and on many emulators a query whose *reply is injected back
into the input stream as if the user had typed it*. A track called `Slipway`
followed by an escape sequence is a track that can redraw the screen around it,
or push characters into the program reading the keyboard.

This is not exotic. It is the cheapest attack a hostile server has against this
program, it needs no vulnerability in any parser, and every field the browse
screens display is a delivery mechanism.

**Therefore: no server-supplied string reaches the terminal without passing
through one function that strips it, and that function lives at the render
boundary in `internal/tui`.**

Two placements were considered. Stripping at the decode boundary in
`internal/subsonic` would be a stronger guarantee — nothing downstream could
ever hold a hostile string — but it silently alters what the server sent. A
track genuinely named with an unusual character comes back changed, and the
`scrobble` sent back to the server would carry the altered form, which makes
shanty lie to the server about its own library. The rendering layer is the
place where the string stops being data and becomes an instruction, so that is
where it stops being an instruction.

**What is stripped:** the C0 controls, DEL, and the C1 range, which is every
byte a terminal can read as the start of a sequence. Newline and tab are
stripped with them; a title is one line and a title containing a newline is a
title that breaks the layout whether or not it was hostile.

**What is not stripped, and why:** the Unicode bidirectional overrides. They
can reorder text visually, which is a real attack in source code, but an artist
name in Arabic or Hebrew is an ordinary thing for this program to display and
mangling it would be a bug affecting real users to prevent a display trick
affecting none. If that changes, it is a later record, not a quiet edit here.

The fake grows a malice mode that returns hostile titles, so this is a property
every screen's golden test carries rather than one function's unit test.

## ADR-014 — mpv has to live in the same mount namespace, which rules out a flatpak

**Status:** accepted. Amends ADR-011, which decided playback is an external
process without saying where that process has to be.

The socket is the point of ADR-011: the credential goes over IPC so it is never
in argv. A unix socket is a filesystem object, so shanty and mpv must see the
same filesystem at the same path. That is an unstated assumption, and it is
load-bearing.

What it rules out:

- **A flatpak mpv.** The sandbox gives the app its own `XDG_RUNTIME_DIR`, so
  the socket shanty creates on the host is not there under the name shanty
  told mpv to use. It fails in a way that looks like a permissions problem.
- **mpv in a container shanty is not in**, and the reverse. On an
  ostree-managed host with a Toolbx for development, that is not a hypothetical
  arrangement — it is the normal one, and both halves have to be on the same
  side of it.

What it requires, per kind of machine: a package manager that writes to the
root shanty runs from. On an ordinary distribution that is the usual command;
on an rpm-ostree host it is `rpm-ostree install mpv` and a reboot; inside a
Toolbx it is the container's own `dnf`, and shanty must run in that container
too.

`doctor` says which, because a fix line that names a command the machine cannot
run is not a fix line -- `dnf install mpv` on a Silverblue host is advice that
fails, and being told the wrong command is worse than being told none.

The alternative was to let mpv live anywhere and pass the stream URL some other
way, which means argv, which is the thing ADR-001 refuses. The constraint is
the price of the property.

---

## Open, and assigned

These are decisions the developer makes, not ones already made. Each becomes a
record above when it is settled — including if it is settled the way the plan
guesses.

| # | question | where it is framed |
|---|---|---|
| ADR-006 | MPRIS, and whether D-Bus fits the §0 budget | PLAN §12 — belongs to v0.3 |
| ADR-012 | The macOS shape of §7 and §8 | above; before darwin is claimed as supported |
| ADR-010 | The measured cost of ADR-003's framework | written when the number is known, not estimated |
