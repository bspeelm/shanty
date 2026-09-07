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

---

## Open, and assigned

These are decisions the developer makes, not ones already made. Each becomes a
record above when it is settled — including if it is settled the way the plan
guesses.

| # | question | where it is framed |
|---|---|---|
| ADR-006 | MPRIS, and whether D-Bus fits the §0 budget | PLAN §12 — belongs to v0.3 |
| ADR-010 | The measured cost of ADR-003's framework | written when the number is known, not estimated |
