# PLAN.md — shanty

*a music player for your own server*

> **shanty** *(n.)* — a rough hut, and a song sung by people working.

shanty is a terminal client for Navidrome and other Subsonic and OpenSubsonic
servers. One binary, mpv for playback, the Subsonic API for everything else.

This document is the plan and the standards in one place. §0 is asserted by CI,
the non-goals in §3 are as binding as the goals, and every standard below names
the thing that enforces it. A standard with nothing enforcing it is a wish.

The rules here were written before the code. Each one exists because it is a
decision that is easy to make silently and expensive to reverse: how a
credential is stored, what a child process can see, what happens to bytes a
server chose. Writing them down first means they are arguments to be won rather
than defaults to be discovered.

---

## §0 Budgets

Asserted by `make budgets`, which runs in `make check`, which gates every
merge. A budget change is a commit to this section with a reason, reviewed like
code. Numbers may start generous and tighten; they may not silently grow.

| budget | limit | how it is counted |
|---|---|---|
| direct Go dependencies | 10 | `go.mod` require block, excluding `// indirect` |
| total modules | 30 | `go list -m all`, minus stdlib and self |
| `panic(` in non-test code | 0 | grep, excluding `_test.go` and vendor |
| binary size (linux_amd64, stripped) | 15 MiB | `make budgets` builds exactly that and measures it |
| packages importing `net/http` | 2 | `internal/subsonic` and its `fake` test server, nothing else |
| test lines : code lines | ≥ 1 : 3 | `wc -l` over `_test.go` vs the rest; a floor, not a target |
| code lines | 6,000 | `cmd` and `internal`, non-test, comments and blanks excluded |
| comment lines : code lines | 25% | the one **hard** ceiling: over budget removes a comment |
| live prose | 4,500 lines | every `.md` outside `docs/history/` and `docs/review/` |
| lockfile-equivalent | go.sum only | no second pinning mechanism; CI builds with `-mod=readonly` |

Two of these behave differently from the rest.

The **comment ratio** is a hard ceiling. Going over it means removing a comment,
not raising the number. It is the only limit here that pushes back on writing
more prose about the code instead of writing less code.

**Live prose** is an absolute line count rather than a share of code, because
this project writes its plan before its code. A ratio would report a project
that planned first as worse than one that did not. 4,500 is 75% of the code
cap.

The dependency budget exists because a terminal interface in Go honestly costs
15 to 25 modules, and the difference between 25 and 250 is entirely a matter of
whether anyone was counting. If a feature does not fit the budget, the feature
waits or the budget change is argued in a decision record — in that order.

---

## §1 What this is

Connect to a server you run, browse what is on it, queue it, play it, tell the
server you played it. The whole program is in service of those five verbs. It
should be one command, sensible on first run, honest when something is broken,
and removable without residue.

The person this is for already runs Navidrome behind HTTPS and works in a
terminal. They do not need onboarding. They need a client that is never the
weakest part of a setup they built carefully.

### The smallest honest alternative

Before the work items: what is the smallest version of this, and why is it
insufficient?

The smallest version is a shell script. Call the Subsonic API with `curl` using
a precomputed token and salt, pick the album out of the response with `jq`, and
give the stream URL to mpv. Perhaps forty lines. It needs no budget, no
decision records and no tests, and it genuinely works — for one of the five
verbs. Playing a known album from a server you already trust is a solved
problem in a shell script, and anyone who only wants that should write one
rather than install this.

It becomes insufficient in two places, and only two, which are therefore the
entire justification for the program.

**Browsing.** Choosing among several thousand artists needs a persistent render
loop over a moving selection. A shell script's answer is either `fzf`, which
hands the problem to someone else's interface and gets no further, or a
reimplementation of one. This is a real cost, but on its own it argues for a
better script rather than a Go program.

**The credential.** This is what settles it. A shell script holding a token
puts that token in `curl`'s arguments on every request, where every other
account on the machine can read it. The same script cannot enforce permissions
on its own credential file, cannot redact a URL before it reaches a log, cannot
limit a response body it is streaming to `jq`, and cannot refuse an option it
does not have. Every property in §5 is one a script of that shape structurally
cannot hold.

So the boundary is narrow and worth stating: shanty is not justified by being
nicer than the script. It is justified by the security properties the script
cannot have, and by browsing. A feature proposed later that serves neither is
argued against by this section.

## §2 Threat model

Recorded in full as ADR-001. Everything else in this plan follows from these
four questions.

**What does a stolen configuration file give an attacker?** With an
OpenSubsonic API key: one revocable string, cancelled from the server. With a
hashed token and salt: a credential that replays against this server only, and
is not the password. With a plain password: whatever else that password opens.
shanty therefore prefers them in that order, and never writes the last one
itself (§6).

**What can another user of the machine see?** The arguments and environment of
every process shanty starts, files readable by others, and anything in a shared
temporary directory. Therefore no credential appears in a child's arguments or
environment (§7), the credential file is 0600 and separate from the settings
(§6), and the socket lives under `XDG_RUNTIME_DIR` in a 0700 directory (§7).
Each of those is a test.

**What can a compromised or faulty server do?** It chooses every byte shanty
parses: JSON, cover art, lyrics, stream data. Therefore response parsing is
fuzzed, image data is size-limited before it reaches a renderer, response
bodies are capped, and stream data goes to mpv. A string from the server never
becomes a filesystem path except through one sanitising function with its own
tests, and never reaches the terminal without passing through another (ADR-013).

**What can the network do?** Delay indefinitely, or lie. Every HTTP client has
a timeout (§9). Certificate verification has no way to be disabled — not a
configuration option, not an environment variable. A user with a self-signed
certificate adds it to their trust store. An `insecure = true` option would be
copied into setup guides and enabled by people who did not know what it turned
off.

## §3 What it is not

A local-files player. A download manager. A Jellyfin client. A daemon. An
auto-updater. A telemetry source. A visualiser. A Lidarr front end. A manager
of anyone's mpv configuration: shanty starts mpv with `--no-config` and
explicit flags, and leaves the user's own setup exactly as it found it.

"A daemon" means a process that outlives what the user asked for: started at
login, run by a service manager, or restarted after it dies. shanty does none
of those. A session left by `:headless` is not one, and ADR-018 says why: it is
created only by a keystroke in a running interface, and it ends by itself when
its queue runs out.

Some of these may become worth doing. Cover art and lyrics are already in §12,
and MPRIS is an open decision. They enter through this document rather than
through a pull request that quietly grows the scope.

## §4 Architecture

```
cmd/shanty/              main, flags, doctor, uninstall
internal/subsonic/       the API client; owns net/http and authentication
internal/subsonic/fake/  in-process Subsonic server, used by every layer's tests
internal/mpv/            one persistent mpv, JSON over a socket, socket lifetime
internal/queue/          play queue, pure functions over data
internal/config/         load, validate, atomic save, permission enforcement
internal/state/          resume position and scrobble backlog; all droppable
internal/tui/            renders state and emits intents; no I/O
docs/decisions.md        decision records, numbered, never deleted
```

Three rules hold that shape, and `make budgets` enforces all three.

`internal/tui` performs no I/O. It renders a model and emits intents, which is
what makes golden-file testing of every screen possible without a server or a
player.

`internal/subsonic` is the only package that imports `net/http`, so everything
shanty sends over the network can be found by reading one package.

`internal/queue` is pure functions over data. Queue behaviour is where music
players accumulate their strangest bugs, and pure code is the cheapest place to
test.

The language is Go. The interface is built with Bubble Tea unless a decision
record says otherwise (ADR-003): it is the best-tested option in this domain,
and `teatest` provides golden-file rendering tests without a harness.

## §5 The standards, each with what enforces it

| standard | enforced by |
|---|---|
| credentials file is 0600, separate from settings | `TestCredentialsPermissions` |
| plain password never written by shanty | `TestNoPlaintextPasswordPersisted`, and the type `Save` writes has no such field |
| no credential in any child's arguments or environment | `TestChildProcessHygiene`, which reads `/proc/<pid>/cmdline` and `environ` |
| writes confined to the four directories | the isolation suite, run against a scratch `$HOME` |
| every HTTP client has a timeout | `TestClientTimeouts`, by reflection over the client |
| no `panic(` outside tests | `make budgets` |
| `internal/tui` does no I/O | `make budgets` greps its imports for `net`, `os`, `os/exec` |
| this project meets the standard it claims | `make standard`, inside `make check` |
| no way to disable certificate verification | a grep for `InsecureSkipVerify` in `make budgets`, and ADR-004 |
| no frame carries a terminal control sequence | `TestNoFrameEverCarriesAnEscape` |
| errors say what happened and what to do | `TestEveryFailureAndWarningCarriesItsFix` |
| CI tokens least-privilege | `permissions: contents: read` at workflow top level; the release workflow alone gets write |
| isolation tests actually ran | the CI job asserts how many passed, not only that none failed |
| releases carry checksums and provenance | goreleaser `checksums.txt` and `actions/attest-build-provenance`; the install script checks the first always and the second on `--verify` |
| the installer asks for what the release publishes | `TestTheInstallerAsksForWhatTheReleaseProduces` |
| documented commands exist and existing commands are documented | `cmd/shanty/docs_test.go` |

## §6 Credentials

Two files, never one. `~/.config/shanty/config.toml` holds the settings and is
0644; nothing in it is secret. `~/.config/shanty/credentials.toml` is 0600 and
holds one of, in order of preference: an OpenSubsonic API key, a precomputed
token and salt, or a `password_file` path pointing at something the user
manages.

shanty uses a plain `password =` line if a user writes one, because refusing
would only move them to a client that accepts one without comment. It never
writes that line itself, and `doctor` names the better option whenever it sees
one.

At every start, loading the credentials checks the file's permissions and
refuses to continue if others can read it, with an error containing the exact
`chmod` to run. Refusing rather than warning: a warning printed once at startup
is easy to miss, and the file stays readable until somebody notices.

The legacy Subsonic `p=` password parameter is never sent, in any mode.
Credential-bearing URLs never reach a log; the debug logger redacts `t`, `s`
and `apiKey` unconditionally, with no setting to turn it off.

## §7 Playback and process hygiene

One mpv, started with `--no-config`, `--idle`, `--no-video`, and a socket at
`$XDG_RUNTIME_DIR/shanty/mpv.sock` in a directory created 0700. Where there is
no runtime directory, the socket goes under the cache directory rather than a
shared temporary one.

Every track is loaded over the socket, so the credential-bearing stream URL
exists in mpv's memory and nowhere another user can read it.
`TestChildProcessHygiene` exists so this cannot be undone sideways, by a future
feature that starts a browser or another helper.

Gapless playback comes from mpv's prefetch: `--prefetch-playlist=yes` and
appending the next queue entry. shanty decodes nothing.

If the socket dies, shanty says so and offers to restart. It does not respawn in
a loop.

## §8 The filesystem contract

```
~/.config/shanty/          settings and credentials, both user-editable
~/.local/state/shanty/     resume position, and the plays a server would not take
~/.cache/shanty/           cover art; deleting it costs bandwidth only
$XDG_RUNTIME_DIR/shanty/   the mpv socket and the control socket, gone at logout
```

One file sits outside that set, and only the installer writes it:

```
$XDG_DATA_HOME/bash-completion/completions/shanty
$XDG_CONFIG_HOME/fish/completions/shanty.fish
```

A shell reads completions from a directory of its own and nowhere else, so a
completion script kept with the four directories would never be read. It is
written by whatever installs the binary, alongside `~/.local/bin/shanty` which
is already outside this set, and `shanty uninstall` removes it. No startup file
is ever written to. `TestIsolationOnlyTheInstallerWritesOutsideTheFourDirectories`
holds this to one command.

Those are the Linux paths. macOS resolves elsewhere: `os.UserConfigDir` is
`~/Library/Application Support` and `os.UserCacheDir` is `~/Library/Caches`,
and there is no `XDG_RUNTIME_DIR`. The table above is one platform's shape of
the promise rather than the promise itself. ADR-012 owes the other, and until
it is answered the isolation suite asserts a different write set per platform.

Nothing else, ever — not `~/.config/mpv`, not the user's shell files, nothing.
The isolation suite runs a first run against a scratch `$HOME` and asserts the
write set matches this list exactly. It also plants files that shanty must not
touch and reads them back unchanged.

All configuration writes are atomic: a temporary file in the destination
directory, permissions set before the rename, and a unique temporary name so
two instances cannot overwrite each other's partial file.

`shanty uninstall` removes all four directories and prints what it removed.

`shanty doctor` checks, in order: mpv present and its version, the runtime
directory usable, the configuration parseable, the credential file's
permissions, the server reachable, the credential accepted, and whether the
server offers a better credential than the one in use. Every failing line
includes the command or configuration change that fixes it. `-json` for
scripting.

## §9 The network contract

shanty talks to exactly one host: the configured server. No Last.fm, no Deezer,
no lyrics services, no update checks.

Play reporting uses the Subsonic `scrobble` endpoint. The server forwards plays
onward if its administrator configured that, which is the right place for it:
credentials for third-party services belong on the server the user already
administers rather than in a second credential store on every client. ADR-005
records this, because "add Last.fm" will be the first request.

Every request sets a `User-Agent` and the Subsonic client name `c=shanty`.
Timeouts are 10 seconds for metadata and 30 for cover art; the stream URL given
to mpv has none, because mpv owns that connection. Response bodies are read
through a capped reader, so a hostile server costs a failed request rather than
the machine's memory.

## §10 Testing

The fake server in `internal/subsonic/fake` is the centre of gravity: an
in-process server speaking enough Subsonic to exercise every path, including
the failure paths — truncated JSON, stalled responses, oversized bodies, path
traversal in filenames, terminal escape sequences in titles, rejected
credentials. Every layer's tests run against it, and nothing in `make check`
touches the network.

The fake also fails the calling test when a request breaks one of shanty's own
rules, so a mistake fails at the place that made it.

Golden files for each screen, so a rendering change is a readable diff.
Property tests on `internal/queue`. Fuzz targets on the JSON decode boundary
and, when it exists, the filename sanitiser, with CI running each corpus
briefly on every merge.

One integration job, CI-only and build-tagged: run the real `deluan/navidrome`
image in a container, seed it with generated silent audio, and drive the whole
sequence — authenticate, browse, play a track to the end through real mpv, and
report the play. The job checks that each subtest passed by name, because a
skipped suite reads exactly like a passing one. This is the test that catches
what the fake cannot: whether shanty is faithful to a real server and a real
player.

## §11 CI and release

`ci.yml` at `permissions: contents: read`, with jobs: `check` (lint, vet,
race-enabled tests, budgets, standard), `isolation` with its ran-count
assertion, `golden`, `fuzz-brief`, `integration`, `buildroot`, and `crossbuild`
across Linux and macOS on both architectures.

Release is goreleaser from a tag: archives, `checksums.txt`, and
`actions/attest-build-provenance` producing a signature with no private key to
store or leak. The release will not build without a review packet in
`docs/review/` for that version.

`bootstrap/install.sh` uses `set -eu` and a temporary working directory, checks
the checksum always, checks provenance with `gh attestation verify` on
`--verify`, and tells the user which of the two it did. The checksum proves the
bytes match the release; the attestation proves who built them.

Dependency freshness is a weekly workflow running `govulncheck` and
`go list -m -u all`, opening or updating one tracking issue. Never an automated
version bump: a client that quietly moves its own dependencies is one whose
build nobody can reproduce.

## §12 Milestones

**v0.1 — the vertical slice.** Configuration and credentials to spec (§6).
Doctor. Browse artists, then albums, then tracks. Play an album through a
persistent mpv with pause, seek, skip and volume. Uninstall. The fake server,
the isolation suite, golden files for the three screens, CI green including the
integration job, and a first tag carrying checksums and a signature — because
adding provenance to a project that already has releases is how it ends up
never being added at all.

Done when a stranger with a Navidrome URL is listening in under two minutes,
and `make check` proves every sentence in §5 through §9 that names a test.

**v0.2 — a daily driver.** Queue semantics, so play-next inserts rather than
replacing. Search. Playlists, read and edited. Starring. Play reporting with an
offline backlog, flushed on reconnect. Resume across devices with
`savePlayQueue` and `getPlayQueue`. Configurable keys.

**v0.3 — the comforts.** Cover art, using the kitty graphics protocol first
with sixel and iTerm2 behind the same interface and always a text fallback.
Synchronised lyrics where the server provides them. Themes as palette files.
MPRIS, if ADR-006 finds room for D-Bus inside the §0 budget.

Deferred indefinitely, per §3: local files, a visualiser, downloads, and
multiple simultaneous servers. Profiles that switch between servers are fine;
two connections at once are not.

## §13 Open decisions

Settled ones are in `docs/decisions.md`. This is a pointer to what is still
open, not a second list.

ADR-006, whether to support MPRIS and whether D-Bus fits the §0 budget. It
belongs to v0.3.

ADR-012, the macOS form of §7 and §8, answered before a release claims to
support macOS.

The minimum mpv version is settled: **0.24.0**. `--input-ipc-server` arrived in
0.17.0 and `--prefetch-playlist` in 0.24.0, so the floor is the later of the
two. Both were read out of mpv's own history rather than guessed: v0.23.0 does
not contain the commit that added prefetching and v0.24.0 does, and there is no
release between them. `doctor` fails an older mpv and names the number.

## §14 On writing things down

Comments say what the code does. Reasoning goes in `docs/decisions.md` and
history goes in commit messages, so a source file is not a changelog with a
compiler.

`docs/decisions.md` only grows. A reversed decision is a new record citing the
old one. When a standard in §5 gains or loses its enforcement, this file changes
in the same commit.

Everything written here is written plainly: complete sentences that state the
subject and the fact. The aim is that a stranger with `grep` and an afternoon
can check what this project claims, rather than having to take it on trust.
