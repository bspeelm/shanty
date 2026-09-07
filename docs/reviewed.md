# Vouching ledger

Who has read what, line by line, and at which commit.

This file exists from the first commit because both sibling projects went
months without one, and nothing said so until a command asked. An empty ledger
is an honest statement of debt; a missing ledger is not a statement at all.

**A ledger entry is not "I skimmed the diff."** It is: a named person read this
file top to bottom, at this commit, and is answerable for what it does.

## Load-bearing surfaces

Named before the code exists, per the standard's §2.3. These are the places a
bug costs the most, and they get human review for their entire lives — not
because the agent writes them badly, but because nobody else will be
answerable for them.

| surface | why it is load-bearing | read at | by |
|---|---|---|---|
| `internal/config` | writes files, sets modes, holds the credential path | — | — |
| `internal/subsonic` | the only package that touches the network; owns auth | — | — |
| `internal/mpv` | spawns a child process; owns the socket and its permissions | — | — |
| `bootstrap/install.sh` | runs unsigned, before anything exists to verify it | — | — |
| `scripts/budgets.sh` | enforces this project's restraint; weakening it is invisible | — | — |
| `.github/workflows/` | holds the release's provenance and its token scopes | — | — |
| `shanty uninstall` | deletes directories | — | — |

Every row is unread. That is the correct state for a project with no code in
it, and the wrong state for one with a release.

## Releases

A review packet per release, in `docs/review/`, answered before the tag. None
yet.

## Staleness

A surface whose file has changed since its ledger entry is stale, and stale
past 30 days is a framework failure rather than a code one — the vouching has
stopped being real. `../agent-context/check.sh` measures it.
