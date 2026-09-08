<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/images/shanty-dark.png">
    <source media="(prefers-color-scheme: light)" srcset="docs/images/shanty-light.png">
    <img alt="shanty — a hut on the shore, drawn in ASCII" src="docs/images/shanty-light.png" width="820">
  </picture>
</p>

<h1 align="center">shanty</h1>

<p align="center"><em>a music player for your own server</em></p>

> **shanty** *(n.)* — a rough hut, and a song sung by people working.

A terminal client for Navidrome and other Subsonic servers. One binary, mpv for
playback, the Subsonic API for everything else.

Connect to a server you run, browse what is on it, queue it, play it, and tell
the server you played it. The whole program is in service of those five verbs,
and things that are not one of them are argued for in
[`docs/decisions.md`](docs/decisions.md) before they are built.

## Installing

shanty needs [mpv](https://mpv.io) installed. It does not decode audio itself —
mpv does that, in its own process, for reasons in
[ADR-011](docs/decisions.md).

From a release:

```sh
curl -fsSL https://raw.githubusercontent.com/bspeelm/shanty/main/bootstrap/install.sh | sh
```

Or from a checkout, which needs no release and no network:

```sh
make install-binary
```

The installer verifies the checksum always, and provenance on `--verify`. The
checksum proves the bytes match what the release published; the attestation
proves who published them, which a checksum cannot.

## Using it

On a machine with nothing configured, `shanty` asks for a server and a
credential and writes both files itself. It takes a password once, stores the
hash the Subsonic protocol sends, and never writes the password down.

```sh
shanty              # browse and play, asking for a server on a first run
shanty setup        # ask again, to change server or credential
shanty doctor       # check the setup; every failure names what to type
shanty doctor -json # the same, for a script
shanty uninstall    # remove the four directories, and say which
shanty version      # what this binary is
shanty help         # the list
```

Move with `↑↓` or `j` `k`, open with `enter`, go back with `esc`. `space`
pauses, `n` and `p` skip, `[` and `]` seek, `+` and `-` change the volume, `q`
quits.

## What it holds itself to

A music client sits between a server you trust and a terminal you use, and
holds a credential the whole time it is running. Most of what shanty promises
is about that position, and each promise names the test that proves it rather
than asking to be believed.

| | |
|---|---|
| Your credential never appears in a command line, where every account on the machine can read it | `TestChildProcessHygiene`, which reads `/proc` |
| The credentials file is 0600, or shanty refuses to start and prints the `chmod` | `TestCredentialsPermissions` |
| shanty never writes a plaintext password — the type it saves has no such field | `TestNoPlaintextPasswordPersisted` |
| Nothing the server sends reaches your terminal as an instruction | `TestNoFrameEverCarriesAnEscape` |
| It writes to four directories, and `uninstall` removes them | the `TestIsolation*` suite, with a ran-count check in CI |
| It talks to your server and nothing else — no update checks, no telemetry | one package imports `net/http`, asserted by `make budgets` |
| TLS verification has no off switch, in any form | a grep in `make budgets`, and [ADR-004](docs/decisions.md) |
| It works against a real server and a real player | the `integration` job: Navidrome in a container, mpv, a track played to the end |

None of that is a novel idea, and the clients that already exist are where the
ideas came from. What shanty does differently is write each one down as a rule
with a command attached, so a stranger can check the claims in an afternoon
instead of taking them on trust.

## Status

| | |
|---|---|
| code | [`cmd/shanty`](cmd/shanty), [`internal/config`](internal/config), [`internal/mpv`](internal/mpv), [`internal/queue`](internal/queue), [`internal/subsonic`](internal/subsonic) and its [`fake`](internal/subsonic/fake), [`internal/tui`](internal/tui) |
| plan | [`PLAN.md`](PLAN.md) — the contract; §0 is asserted by `make budgets` |
| what it is for | [`docs/north-star.md`](docs/north-star.md) |
| decisions | [`docs/decisions.md`](docs/decisions.md) — ADR-001 is the threat model, written first |
| reviews | [`docs/review/`](docs/review) — a packet per release, saying what was checked and what was not |

Browsing and playback work. Search, playlists, star, an offline scrobble
backlog and cross-device resume are next; cover art and lyrics after that. The
milestones are in [`PLAN.md`](PLAN.md) §12 and the refusals in §3.

## Working on it

```sh
make check      # lint, vet, race, budgets, standard - the gate
make budgets    # the §0 numbers, on their own
make standard   # conformance against the development standard
```

`make budgets` fails the moment a rule is broken — verified by planting a
`panic` and an `InsecureSkipVerify` and watching it refuse both. That the line
above still lists what `make check` actually runs is verified too, by
`TestTheDocumentedGateMatchesTheMakefile`: this table drifted from the Makefile
within a single commit, which is what a prose compiler is for.

Nothing in `make check` touches the network. The fake server in
[`internal/subsonic/fake`](internal/subsonic/fake) stands in for a real one and
refuses requests that break shanty's own promises, so a mistake fails the test
that made it rather than shipping.

## Maintenance

Written and maintained by one person, for their own use, and maintained while
that stays true. There is no team behind this and no support commitment. Each
release carries a packet in [`docs/review/`](docs/review) saying what was
checked and, more usefully, what was not.

## Licence

MIT — [`LICENSE`](LICENSE), ADR-008.
