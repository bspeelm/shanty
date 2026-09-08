# Working on shanty

## Building and testing

```sh
make check      # lint, vet, race, budgets, standard - the gate
make test       # go test
make race       # go test -race
make budgets    # the size and dependency limits, on their own
make standard   # conformance against the development standard
make build      # build ./shanty for this machine
make install-binary  # build it and put it on PATH
make crossbuild # every platform a release ships
```

`make check` is what CI runs and what has to pass before a commit.

Nothing in `make check` touches the network. Every layer's tests run against an
in-process Subsonic server in `internal/subsonic/fake`, which also refuses
requests that break the client's own rules, so a mistake fails the test that
made it.

One suite is excluded from the fast path and needs a container runtime, a
network and mpv:

```sh
go test -tags=integration ./cmd/shanty/ -v -timeout=10m
```

It runs a real Navidrome in a container and plays a real track through real
mpv. It is the only test that can tell you whether mpv accepts the flags this
program gives it.

## The packages

| | |
|---|---|
| [`cmd/shanty`](cmd/shanty) | commands, and the loop that connects the screen to the server and the player |
| [`internal/config`](internal/config) | the two files, their permissions, and the four directories |
| [`internal/subsonic`](internal/subsonic) | the API client — the only package that talks to the network |
| [`internal/subsonic/fake`](internal/subsonic/fake) | an in-process server for tests |
| [`internal/mpv`](internal/mpv) | one mpv, its socket, and its lifetime |
| [`internal/queue`](internal/queue) | what plays next; pure functions over data |
| [`internal/tui`](internal/tui) | the screens; renders and emits intents, does no I/O |

Three rules hold that shape, and `make budgets` enforces all three: `internal/tui`
does no I/O, `internal/subsonic` is the only package importing `net/http`, and
`internal/queue` stays free of both.

## Limits

`make budgets` fails on any of: more than 10 direct dependencies or 30 modules,
more than 6,000 lines of code, comments above 25% of code, prose above 4,500
lines, a binary over 15 MiB, more than two packages importing `net/http`, any
`panic` outside tests, or any way to disable certificate checking.

They are not suggestions. Changing one is a change to `PLAN.md` §0 with a
reason, reviewed like code.

## Before you write anything

Read [`docs/north-star.md`](docs/north-star.md), [`PLAN.md`](PLAN.md), and the
titles in [`docs/decisions.md`](docs/decisions.md). The decision log records
what was refused as well as what was chosen, so a proposal that has already
been declined gets a number rather than a fresh argument.

Every release carries a packet in [`docs/review/`](docs/review) saying what was
checked and what was not.

## Documentation

The [wiki](https://github.com/bspeelm/shanty/wiki) lives in
[`wiki/`](wiki) and is published from there on merge. Edit the files, not the
published pages.
