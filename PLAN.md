# PLAN.md — shanty

*a music player for your own server, built the way bothy is built*

> **shanty** *(n.)* — a rough hut, and a song sung by people working.
> Working name; naming is ADR-002 and the dev may well do better.

shanty is a terminal client for Navidrome and other Subsonic/OpenSubsonic
servers. One binary, mpv for playback, the Subsonic API for everything else.
This document is the plan and the standards in one place, in the same shape as
bothy's own PLAN.md: §0 is asserted by CI, the non-goals are as binding as the
goals, and every standard below names the gate that enforces it. A standard
without a gate is a wish.

Context the dev should have: this plan came out of a code audit of five
existing Subsonic TUIs (must, SubTUI, Navidrome-tui, nd, termsonic) scored
against bothy's practices. None passed. The common failures were: zero or
near-zero tests, credentials in world-readable files, auth tokens leaking into
process argv, no release provenance, and no written reasoning anywhere. Each
of those failures appears below as a rule with a test attached, because every
one of them was a decision someone made silently. We make them out loud.

---

## §0 Budgets

Asserted by `make budgets`, which runs in `make check`, which gates every
merge. A budget change is a commit to this section with a reason, reviewed
like code. Numbers may start generous and tighten; they may not silently grow.

| budget | limit | how it is counted |
|---|---|---|
| direct Go dependencies | 10 | `go.mod` require block, excluding `// indirect` |
| total modules | 30 | `go list -m all`, minus stdlib and self |
| `panic(` in non-test code | 0 | grep, excluding `_test.go` and vendor |
| binary size (linux_amd64, stripped) | 15 MiB | `make budgets` builds exactly that and measures it |
| packages importing `net/http` | 2 | `internal/subsonic` and its `fake` test server, nothing else |
| test lines : code lines | ≥ 1 : 3 | `wc -l` over `_test.go` vs the rest; a floor, not a target |
| code lines | 6,000 | `cmd` and `internal`, non-test, comments and blanks excluded |
| comment lines : code lines | 25% | the one **hard** ceiling: over budget retires a comment |
| live prose | 4,500 lines | every `.md` outside `docs/history/` and `docs/review/` |
| lockfile-equivalent | go.sum only | no second pinning mechanism; `go.sum` is the lockfile and CI builds with `-mod=readonly` |

Two of these need their reasoning stated, because their shape differs from
bothy's. The **comment ratio** is hard where the others are soft: over budget
means retiring a comment, never raising the number, since an agent produces
prose the way a fire produces smoke and this is the only line in the file that
pushes back. **Live prose** is an absolute count rather than bothy's
share-of-code, because this project writes its plan in full before its code — a
ratio would report a project that planned first as worse than one that did not.
4,500 is 75% of the code cap: the same number bothy's rule produces at the size
shanty says it will stop growing at.

Rationale for the dependency budget: the existing field ranges from nd's 19
modules to Navidrome-tui's 599 crates. A Bubble Tea TUI honestly costs
15–25 modules; the budget forces each addition past a human. If a feature
cannot fit the budget, the feature waits or the budget change gets argued in
an ADR — in that order.

---

## §1 What this is

Connect to a server you run, browse what is on it, queue it, play it, tell the
server you played it. The whole program is in service of those five verbs. It
should feel like bothy feels: one command, sensible on first run, honest when
something is broken, and removable without residue.

The user this is for already runs Navidrome behind HTTPS and lives in a
terminal. They do not need onboarding; they need the client to never be the
weakest link in their setup.

### The smallest honest alternative

Before the work items, the question the agent will never volunteer: what is the
smallest version of this, and why is it insufficient?

The smallest version is a shell script. `curl` the Subsonic API with a
precomputed token and salt, `jq` the album out of the response, and hand the
stream URL to `mpv`. Perhaps forty lines. It needs no budget, no ADRs and no
tests, and the honest admission is that **it works** — for one of the five
verbs. Playing a known album from a server you already trust is genuinely a
solved problem in a shell script, and anyone who only wants that should write
one rather than install this.

It becomes insufficient in two places, and only two, which is why they are the
entire justification for the program.

**Browse.** Choosing among several thousand artists is a persistent render loop
over a moving selection, and a shell script's answer to that is either `fzf` —
which hands the problem to someone else's TUI and gets no further — or a
reimplementation of one. This is a real cost, but on its own it would argue for
a nicer script, not a Go program.

**The credential.** This is the one that settles it. A shell script holding a
token puts that token in `curl`'s argv on every single request, where every
other account on the machine reads it out of `ps` — which is exactly the failure
ADR-001 names and `TestChildProcessHygiene` exists to prevent. The same script
cannot enforce a mode on its own credential file, cannot redact a URL before it
reaches a log, cannot cap a response body it is streaming to `jq`, and cannot
refuse a knob it does not have. Every property in §5 is a property a script of
this shape structurally cannot hold.

So the fence is narrow and worth stating plainly: shanty is not justified by
being nicer than the script. It is justified by the four security properties the
script cannot have, and by browse. If a feature proposed later serves neither of
those, this section is the argument against it.

## §2 Threat model — becomes ADR-001, written before code

The first commit of substance is `docs/decisions.md` with this in it,
expanded. Everything else in the plan hangs off these four questions.

**What does a stolen config yield?** With OpenSubsonic API-key auth: one
revocable key, revoked server-side in one click. With hashed token+salt: a
replayable credential for this server only, but not the password itself, so
nothing reused elsewhere falls. With a plaintext password: whatever the user
reused it for. Therefore the client prefers these in exactly that order, and
the worst mode is never written to disk by shanty itself (§6).

**What can another local user see?** Process argv and environment of every
process we spawn, world-readable files, and anything in a shared /tmp.
Therefore: tokens never appear in argv or child environment (§7), credentials
are 0600 in their own file (§6), and sockets live under `XDG_RUNTIME_DIR`
in a 0700 directory (§7). Each of those three sentences is a test.

**What can a malicious or compromised server do to us?** It controls every
byte we parse: JSON responses, cover art, lyrics, stream data. Therefore
response parsing is fuzzed against the decoder, image data is decoded with
size limits before it reaches the renderer, download sizes are capped, and
stream data goes to mpv, whose job that is. The server names files; we never
turn a server-supplied string into a filesystem path without sanitising it
through one function with its own tests.

**What can the network do?** Delay forever or lie. Every HTTP client has a
timeout (§9). TLS verification has no off switch — not a config option, not
an env var. A user with a self-signed cert adds it to their trust store; an
`insecure = true` knob is one paste away from being cargo-culted into every
setup guide on the internet. This is an ADR, because people will ask.

## §3 What it is not

A local-files player. A download manager. A Jellyfin client. A daemon. An
auto-updater. A telemetry source. A visualizer. A Lidarr frontend. A manager
of anyone's mpv config — shanty launches mpv with `--no-config` and explicit
flags, leaving the user's own mpv exactly as it found it, the same custom
bothy keeps toward dotfiles.

Some of these may become worth doing (art and lyrics are already in §12;
MPRIS is an open ADR). They enter through this document, not through a pull
request that quietly grows the scope.

## §4 Architecture

```
cmd/shanty/          main, flags, doctor, uninstall
internal/subsonic/   the API client. Owns net/http. Owns auth.
internal/subsonic/fake/  in-process httptest Subsonic server, used by every layer's tests
internal/mpv/        one persistent mpv, JSON IPC, socket lifecycle
internal/queue/      play queue semantics, pure and unit-tested
internal/config/     load, validate, atomic save, permission enforcement
internal/state/      resume data, scrobble backlog; cache, all droppable
internal/tui/        Bubble Tea. Renders state, emits intents. No I/O.
docs/decisions.md    ADRs, numbered, never deleted
```

Three load-bearing rules. First, `internal/tui` does no I/O — it renders a
model and emits intents, which is what makes golden-file testing of every
screen possible and cheap. Second, `internal/subsonic` is the only package
allowed to import `net/http` (§0 asserts it), so "what does this program say
on the network" has a one-package answer, the way bothy confines fetching to
`internal/fetch`. Third, `internal/queue` is pure functions over data —
queue behaviour is where music players accumulate their weirdest bugs, and
pure code is where tests are cheapest.

Language is Go, matching bothy's idiom and toolchain so one dev carries one
mental model across both projects. TUI framework is Bubble Tea unless the dev
ADRs otherwise (ADR-003): it is the best-tested path in this domain and
`teatest` gives golden-file rendering tests out of the box.

## §5 The standards, each with its gate

Prose here, enforcement in the named place. This table is the contract; the
audit that produced it found every row violated by at least one shipping
client.

| standard | gate |
|---|---|
| credentials file is 0600, separate from settings | `TestCredentialsPermissions` in the isolation suite |
| plaintext password never written by shanty | `TestNoPlaintextPasswordPersisted` |
| no token in any child argv or env | `TestChildProcessHygiene` reads `/proc/<pid>/cmdline` and `environ` of the spawned mpv |
| writes confined to config, state, and runtime dirs | isolation suite runs the app against a scratch `$HOME` and diffs the filesystem |
| every HTTP client has a timeout | `TestClientTimeouts` via reflection over constructed clients |
| no `panic(` outside tests | `make budgets` |
| `internal/tui` does no I/O | `make budgets` greps its import graph for `net`, `os`, `os/exec` |
| this project meets the standard it claims | `make standard`, inside `make check` |
| no TLS-verification bypass exists | grep gate for `InsecureSkipVerify` in `make budgets`, plus ADR-004 saying why |
| errors say what happened and what to do | review standard, with bothy's error voice as the reference: "the release was changed, or the download is corrupt; nothing was installed" |
| CI tokens least-privilege | `permissions: contents: read` at workflow top level; release workflow alone gets write |
| isolation tests actually ran | CI job asserts `--- PASS` count ≥ N, exactly bothy's drifted-filter guard |
| releases carry checksums and provenance | goreleaser `checksums.txt` + `actions/attest-build-provenance`; install script verifies the first always and the second on `--verify` |
| reasoning is written down | ADR per irreversible decision; `make budgets`; the house style is bothy's 25% of code, against the ~1% typical of this field |

## §6 Credentials

Two files, never one. `~/.config/shanty/config.toml` holds settings and is
0644 — nothing in it is secret. `~/.config/shanty/credentials.toml` is 0600
and holds exactly one of, in order of preference: an OpenSubsonic API key
(Navidrome supports these; `shanty doctor` should detect server support and
say so), a precomputed md5 token+salt pair, or a `password_file` path pointing
at something the user manages — an agenix secret, a pass(1) entry via process
substitution, whatever. shanty will *use* a plaintext `password =` line if a
user writes one by hand, because refusing would just push people to the worst
client available, but it never writes that line itself, and `doctor` names the
better option when it sees it.

On every start, config loading checks the credential file's mode and refuses
to proceed past 0600 with an error that includes the exact `chmod` to run.
Refusing, not warning: the audit found the field's best-engineered client
shipping a world-readable password for want of exactly this check.

The legacy Subsonic `p=` password parameter is never sent, in any mode.
Auth-bearing URLs never reach a log; the debug logger redacts `t`, `s`, and
`apiKey` query parameters unconditionally — there is no
`redact = false` setting, because a knob that exists gets turned.

## §7 Playback and process hygiene

One mpv, spawned at startup with `--no-config`, `--idle`, `--no-video`, and an
IPC socket at `$XDG_RUNTIME_DIR/shanty/mpv.sock` inside a directory created
0700 (falling back to `os.UserCacheDir` — never a shared /tmp — when the
runtime dir is absent). Every track load goes over IPC with `loadfile`, so the
token-bearing stream URL exists in mpv's memory and nowhere inspectable. The
audit found one client passing these URLs in argv, visible to every local user
in `ps`; `TestChildProcessHygiene` exists so that mistake cannot be
reintroduced sideways, for instance by a future "open in browser" feature.

Gapless playback comes from mpv's prefetch (`--prefetch-playlist=yes` and
appending the next queue entry), not from shanty decoding anything. Crash
recovery: if the socket dies, shanty says so and offers restart; it does not
silently respawn in a loop.

## §8 The filesystem contract

```
~/.config/shanty/    settings, credentials, both user-editable
~/.local/state/shanty/   resume position, scrobble backlog
~/.cache/shanty/     cover art cache; deleting it costs bandwidth only
$XDG_RUNTIME_DIR/shanty/  the mpv socket, gone at logout
```

Those are the Linux paths, and they are the contract there. macOS resolves
elsewhere — Go's `os.UserConfigDir` is `~/Library/Application Support` and
`os.UserCacheDir` is `~/Library/Caches`, and there is no `XDG_RUNTIME_DIR` at
all — so the table above is one platform's shape of a promise, not the promise.
ADR-012 owes the other, and until it is answered the isolation suite asserts a
write set that differs by platform.

Nothing else, ever — not `~/.config/mpv`, not the user's shell files, nothing.
The isolation suite runs shanty's install-free first run against a scratch
HOME and asserts the write set matches this list exactly, which is bothy's
ADR-009 discipline transplanted. All config writes are atomic:
`os.CreateTemp` in the destination directory, mode set before rename, unique
temp names so two instances cannot eat each other's half-written file —
lifted straight from bothy's `stageExecutable`, comment and all.

`shanty uninstall` removes all four directories and prints what it removed.
`shanty doctor` checks, in order: mpv present and ≥ the minimum version, the
runtime dir usable, config parseable, credential file mode, server reachable,
auth valid, and whether the server offers a better auth mode than the one in
use. Every failure line includes the command or config change that fixes it.
`--json` for scripting, matching bothy's.

## §9 The network contract

shanty talks to exactly one host: the configured server. No Last.fm, no
Deezer, no lyrics services, no update checks. Scrobbling is the Subsonic
`scrobble` endpoint — the server federates outward if the user set that up,
which is the correct trust boundary: secrets for third-party services belong
on the server the user already administers, not in a second credential store
on every client. This is ADR-005, because "add Last.fm" will be the first
feature request.

All requests set a `User-Agent` and the Subsonic `c=shanty` client name.
Timeouts: 10s metadata, 30s cover art, none on the stream URL handed to mpv
(mpv owns that connection). Response bodies read through `io.LimitReader`
with per-endpoint caps, because a malicious server should cost us a failed
request, not the machine's memory.

## §10 Testing

The fake server in `internal/subsonic/fake` is the center of gravity: an
in-process `httptest` server speaking enough Subsonic to exercise every code
path, including the error and malice paths — wrong checksums of reality like
truncated JSON, 30-minute stalls (`context` cancellation is part of the API
surface), oversized art, path-traversal filenames, auth failures. Every
layer's tests run against it; nothing in `make check` touches the network,
the same property bothy's container-tagged tests protect.

Golden tests via `teatest` for each screen, so a rendering regression is a
readable diff. Property tests on `internal/queue` (shuffle preserves the
multiset, resume round-trips). Fuzz targets on the JSON decode boundary and
the filename sanitiser — `go test -fuzz` targets committed, and CI runs each
corpus briefly on every merge.

One integration job, CI-only and build-tagged like bothy's container suite:
docker-run the real `deluan/navidrome` image, seed it with three
generated-silence FLACs, and drive the true vertical slice — auth, browse,
stream a track to completion through real mpv, scrobble, resume. The job
greps its own log to assert each subtest ran. This is the test that catches
what the fake cannot: our fidelity to the real server.

## §11 CI and release

`ci.yml` at `permissions: contents: read` with a comment saying why, jobs:
`check` (`make check` = lint, vet, race-enabled tests, budgets), `isolation`
(with the ran-count assertion), `golden`, `fuzz-brief`, `integration`
(navidrome-in-docker, tagged), `crossbuild` (linux/darwin, amd64/arm64). A
`scope` job may later skip heavy jobs for docs-only changes; correctness
before speed.

Release is goreleaser from a tag: archives, `checksums.txt`, and
`actions/attest-build-provenance` producing a Sigstore attestation — six
lines of YAML for the one standard no client in this field currently meets.
`bootstrap/install.sh` follows bothy's line for line where it applies:
`set -eu`, mktemp workdir, checksum verification always, `--verify` doing
`gh attestation verify`, and the honest distinction printed to the user —
the checksum proves the bytes match the release, the attestation proves who
built them. `go install github.com/<owner>/shanty/cmd/shanty@<tag>` is the
second path and needs nothing from us; go.sum does the verifying.

Dependency freshness follows bothy's `outdated` philosophy exactly: a weekly
workflow running `govulncheck` and `go list -m -u all`, opening or updating
one tracking issue. Never an automated bump PR — an installer that quietly
moves its own pins is an installer whose output nobody can reproduce, and a
client that quietly moves its dependencies is no different.

## §12 Milestones

**v0.1 — the vertical slice.** Config and credentials to spec (§6). Doctor.
Browse artists → albums → tracks. Play an album through persistent mpv with
pause, seek, skip, volume. Uninstall. The fake server, the isolation suite,
golden tests for the three screens that exist, CI fully green including
integration, and the first tag releases with checksums and attestation —
provenance ships with the *first* release, because retrofitting it later is
how the other projects ended up without it. Definition of done: a stranger
with a Navidrome URL is listening in under two minutes, and `make check`
proves every sentence in §5 through §9 that has a test named for it.

**v0.2 — a daily driver.** Queue with sane semantics (play-next slots in,
does not destroy the queue). search3. Playlists read and edit. Star/unstar.
Scrobble with an offline backlog in state, flushed on reconnect.
savePlayQueue/getPlayQueue resume across devices. Keybind config.

**v0.3 — the comforts.** Cover art, kitty graphics protocol first, sixel and
iTerm2 behind the same interface, always with the text fallback. Synced
lyrics where the server provides them. Themes as palette files, bothy-style:
shanty ships one open palette and generates the rest, so anything the user
licensed stays on their machine. MPRIS if ADR-006 admits the D-Bus
dependency inside the §0 budget.

Deferred indefinitely, per §3: local files, visualizer, downloads, multiple
simultaneous servers (profiles that switch, yes; two connections at once, no).

## §13 Open ADRs, assigned to the dev

Settled ones live in `docs/decisions.md`; this is what is still open. It is a
pointer, not a second list — the log is the record.

ADR-006 MPRIS, and whether D-Bus fits the §0 budget: v0.3, not before.
ADR-012 the macOS shape of §7 and §8, answered before darwin is claimed as a
supported platform rather than after. And one number with no record yet: the
**minimum mpv version**, which §8 has `doctor` checking against something
nobody has chosen. `--input-ipc-server` and `--prefetch-playlist=yes` each have
a floor; the floor is whichever is later, established by reading mpv's
changelog rather than by guessing.

## §14 On writing things down

Every non-obvious block gets the comment bothy would give it: not what the
code does, but what it refuses to do and why — "a corrupt download never
lands on disk at all" is the register. decisions.md grows monotonically;
a reversed decision is a new ADR citing the old one. When a standard in §5
gains or loses a gate, this file changes in the same commit. The audit that
started this project could score five codebases in an afternoon *only*
because bothy wrote its reasoning down and the others did not; shanty should
be auditable the same way by a stranger with grep and an afternoon.

---

*Hand this to the structure: repo scaffold, ADR-001, the fake server, and the
§0 `make budgets` target come first — in that order, because the budgets only
mean something if they are asserting from the first commit.*
