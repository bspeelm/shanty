# North star

*A client that is never the weakest link in a setup its user already trusts.*

## The invariant

Five verbs, and the whole program is in service of them:

```
connect  →  browse  →  queue  →  play  →  tell the server you played it
```

Connect to a server you run. Browse what is on it. Queue it. Play it. Scrobble
it back. Everything else is a comfort, and comforts are argued for in
`docs/decisions.md` before they are built.

## Who it is for

Someone who already runs Navidrome behind HTTPS and lives in a terminal. They
do not need onboarding. They need a client that does not undo the care they put
into the server — which is the entire product, stated as a person.

That is a narrow audience on purpose, and the narrowness is what makes the
refusals in §3 of the plan cheap to keep.

## What "good" means here

It should feel the way bothy feels: one command, sensible on first run, honest
when something is broken, and removable without residue.

Concretely, and each of these is a test rather than an aspiration:

- **One command.** A stranger with a server URL is listening in under two
  minutes.
- **Honest when broken.** Every failure line carries the command or the config
  change that fixes it. `doctor` says what is wrong and what to type.
- **No residue.** Four directories, listed in the plan's §8, and nothing else
  ever. The isolation suite runs a first run against a scratch `$HOME` and
  asserts the write set matches that list exactly.
- **Nothing of anyone else's is touched.** mpv is launched with `--no-config`
  and explicit flags; the user's own mpv setup is left exactly as it was found.

## What would mean this failed

Not "it has few users" — the audience is small by construction. These:

- A credential of any kind ends up somewhere world-readable, or in an argv.
- A shipped release cannot be traced to the workflow that built it.
- Someone reads the code and cannot tell why a decision was made, because it
  was made silently.

The audit that started this project scored five existing clients in an
afternoon, and could only do that because one project in this family had
written its reasoning down and the others had not. Being auditable that way by
a stranger with grep and an afternoon is the standard being aimed at.

## The scope fence

`PLAN.md` §3 holds the not-list, and it is as binding as the goals. Things on
it enter through that document, never through a pull request that quietly grows
what the program is.
