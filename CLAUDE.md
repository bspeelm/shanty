# Working on shanty

A terminal client for Navidrome and other Subsonic servers. One binary, mpv for
playback, the Subsonic API for everything else.

## Read first, every session

`docs/north-star.md`, `PLAN.md`, and the ADR titles in `docs/decisions.md`. The
recorded refusals ride along for free — a decision already declined is settled,
and a re-proposal is answered with its ADR number rather than a fresh argument.

`PLAN.md` is the contract. §0 is asserted by `make budgets`, the non-goals in
§3 are as binding as the goals, and every standard in §5 names the gate that
enforces it. **A standard without a gate is a wish** — if you add one, add its
gate in the same commit.

## Comments describe what the code does

A comment says what a function, type or field is and what it does. It does not
explain why the code was written that way, what was considered instead, or what
it used to be. Reasoning belongs in `docs/decisions.md`, and history belongs in
commit messages.

If a comment is arguing for a decision, it is in the wrong file.

Corollaries:

- Write plainly. Complete sentences that state the subject and the fact. No
  aphorisms, no inversion for effect, no sentences built to land rather than to
  inform. This applies to every artefact: comments, commit messages, decision
  records, the README and the wiki.
- Never reference another project by name in code, comments or documentation.
- A bug fix ships with a test or a doctor check, and the test name carries the
  story.
- Prefer deleting a comment to writing one that restates the line below it.

**User-facing prose says what the program does.** The README and the wiki
describe behaviour: what happens, what to type, what to expect. Nothing about
how the project was built, why a decision was taken, or which test proves what.

## Everything else

- **Dependencies are budgeted, not forbidden**: 10 direct, 30 modules. Adding
  one is an ADR, argued before it is added, not after.
- `make check` before every commit: lint, vet, race tests, budgets. Do not
  argue with a budget; change `PLAN.md` §0 and say why.
- Conventional commits. `main` requires a PR.
- Every user-facing claim names the test or CI job that proves it, or the claim
  goes.
- A new result in a closed-world set goes in the expectation table. The test
  failure will say so.
- Nothing in `make check` touches the network. The fake server in
  `internal/subsonic/fake` is the centre of gravity; the one job that talks to
  a real Navidrome is build-tagged and CI-only.

## The three architectural rules

These are not style. They are what make the tests cheap, and breaking one
quietly is how this project stops being testable:

1. **`internal/tui` does no I/O.** It renders a model and emits intents. That
   is what makes golden-file testing of every screen possible.
2. **`internal/subsonic` is the only package that imports `net/http`.**
   `make budgets` asserts it. "What does this program say on the network" must
   have a one-package answer.
3. **`internal/queue` is pure functions over data.** Queue behaviour is where
   music players accumulate their strangest bugs, and pure code is where tests
   are cheapest.

## Instinct fences

Mine to self-check. Each is a failure that has actually shipped in agent-led
work, so the instinct is named before it fires.

- **Answer the ask.** Extra scope is proposed in prose, never shipped in the
  diff.
- **Fix in place.** No `_v2`/`_new`/`_old` parallel files; a replacement and
  the deletion it replaces travel in one commit.
- **Never discard an error** without a comment saying why ignoring it is
  correct.
- **Never weaken or delete a test to go green.** A failing test is information;
  report what it told you.
- **Never claim something ran** without the command and its output. Anything
  unverified is labelled unverified — including in reports about my own work.
- **Test the code, not the machine.** A test that asks whether something is
  installed passes where it was written and fails where the artifact is built.
- **Done is the consumed artefact.** A green tag is a step; the release
  downloaded and verified is finished.
- **No local detail leaves the machine.** Paths, directory layout, account
  names and local naming never reach an issue, a PR, a commit message or a doc.
- **No placeholders** without a named tracking issue. The TODO count may not
  grow; it is 0.
- **One diff, one intent.** Refactors and fixes ship separately.
- **Give the strongest counter-argument first** on any proposal, and build only
  what survives it. Direction is settled by ADR, not by enthusiasm.
- **Polish is not proof.** The existence of docs, tests or ADRs is never
  offered as evidence of quality. Only executed checks count.
- **A knob that exists gets turned.** Before adding a setting that weakens
  something, read ADR-001 and consider that it will end up in a setup guide.
- **Open the pull request when the work is done** and the checks are green, not
  before. Do not keep pushing to a PR under review.
