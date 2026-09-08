# shanty

*a music player for your own server*

> **shanty** *(n.)* — a rough hut, and a song sung by people working.
> Working name; the name is ADR-002 and is not settled.

A terminal client for Navidrome and other Subsonic/OpenSubsonic servers. One
binary, mpv for playback, the Subsonic API for everything else.

Connect to a server you run, browse what is on it, queue it, play it, and tell
the server you played it. The whole program is in service of those five verbs.

**The client is not written yet.** What exists is the plan, the decisions taken
before any code, the commands that enforce them, the fake Subsonic server every
other layer's tests will run against, and the config and credential layer. That order is deliberate: budgets
introduced after a project is over them are not budgets, and a test double
introduced after the code it doubles for is a description of that code rather
than a check on it.

## Status

| | |
|---|---|
| code | [`cmd/shanty`](cmd/shanty), [`internal/config`](internal/config), [`internal/mpv`](internal/mpv), [`internal/queue`](internal/queue), [`internal/subsonic`](internal/subsonic) and its [`fake`](internal/subsonic/fake), [`internal/tui`](internal/tui) |
| plan | [`PLAN.md`](PLAN.md) — the contract; §0 is asserted by `make budgets` |
| what it is for | [`docs/north-star.md`](docs/north-star.md) |
| decisions | [`docs/decisions.md`](docs/decisions.md) — ADR-001 is the threat model, written first |
| who has read what | [`docs/reviewed.md`](docs/reviewed.md) — empty, and honest about it |

shanty needs [mpv](https://mpv.io) installed; it does not decode audio itself
([ADR-011](docs/decisions.md)).

```sh
curl -fsSL https://raw.githubusercontent.com/bspeelm/shanty/main/bootstrap/install.sh | sh
```

The installer verifies the checksum always, and provenance on `--verify`. The
checksum proves the bytes match what the release published; the attestation
proves who published them, which a checksum cannot.

```sh
shanty              # browse and play
shanty doctor       # check the setup; every failure names what to type
shanty doctor -json # the same, for a script
shanty uninstall    # remove the four directories, and say which
shanty version      # what this binary is
shanty help         # the list
```

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

## Why another one

Five existing Subsonic clients were read and scored before this was started.
None passed. The common failures were near-zero tests, credentials in
world-readable files, auth tokens visible in process arguments to every account
on the machine, no release provenance, and no written reasoning anywhere.

Each of those is a rule in `PLAN.md` §5 with a test attached, because every one
of them was a decision somebody made silently. This project makes them out
loud, which is also the only reason those five could be scored in an afternoon:
one project in this family wrote its reasoning down.

## Maintenance

Written and maintained by one person, for their own use, and maintained while
that stays true. There is no team behind this and no support commitment; the
ledger in `docs/reviewed.md` shows what has actually been reviewed rather than
what is claimed.

## Licence

MIT — [`LICENSE`](LICENSE), ADR-008.
