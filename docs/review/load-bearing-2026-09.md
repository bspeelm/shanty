# Reading guide: the three load-bearing packages

Not a release packet — v0.1 has not shipped. This is the same shape, early,
because these packages are about 620 lines today and will not be smaller again.

**Read this with suspicion.** §7.5 asks for the "what could go wrong" cards to
be drafted by a model other than the one that wrote the code. They were not:
the author wrote them, and a guided tour shows the good rooms. Treat the cards
as the floor. At least one finding should come from outside them, and if the
read turns up nothing outside these cards, record that as a finding about the
tour rather than as evidence about the code.

Fill in `docs/reviewed.md` when done. An entry means: you read the file top to
bottom at this commit and are answerable for what it does.

---

## internal/config — 250 lines

**What it does now.** Owns every file shanty reads or writes. Resolves four
directories from the environment. Reads and writes two files: `config.toml` at
0644 with nothing secret in it, and `credentials.toml` at 0600 with exactly the
secret. Refuses to start if the credentials file is readable by anyone else.
Never writes a plaintext password.

**What could go wrong.**

- *The permission check is one bitmask.* `perm &^ CredentialsMode != 0` in
  `LoadCredentials`. Get that expression wrong and a world-readable credential
  is accepted silently, which is the exact failure the audit found shipping.
  Read the expression, not the test.
- *Mode is set before rename, in `writeAtomic`.* Reverse those two lines and
  there is a window where `credentials.toml` exists at 0644. Nothing would
  fail; the file ends up correct.
- *`Paths.All()` is the write set.* `uninstall` deletes exactly these and the
  isolation suite asserts nothing lands outside them. A fifth directory
  resolved anywhere in this package and left out of `All()` is one that
  uninstall leaves behind and no test looks for.
- *`ResolvePassword` skips the mode check for non-regular files*, so a `pass(1)`
  entry arriving as a pipe is read without one. Decide whether you agree that a
  pipe's permissions are not what protects it.
- *`EnsureRuntime` chmods a directory it did not create.* If something else got
  there first and is owned by another account, this fails rather than
  proceeding — check that reads as the right choice to you.

**How to check it.**

```sh
go test ./internal/config/ -v          # expect 53 PASS lines, subtests included
```

Then break it deliberately and confirm the tests object:

```sh
# accept a group-readable credential
sed -i 's/perm&^CredentialsMode != 0/false/' internal/config/credentials.go
go test ./internal/config/ -run CredentialsPermissions   # expect FAIL on 0640 and 0604
git checkout internal/config/credentials.go
```

---

## internal/subsonic — 330 lines

**What it does now.** The only package that imports `net/http`; `make budgets`
asserts that. Builds authenticated request URLs, decodes responses, caps how
much it will read, and redacts credentials before anything is logged. Also
builds the stream URL that is handed to mpv over the socket.

**What could go wrong.**

- *`Redact` works from a list of parameter names.* Add a fourth authentication
  mode with a new parameter and it is logged in clear until someone remembers
  this list. The test checks the list against what today's three constructors
  emit, which is the best it can do — it cannot know about a mode nobody wrote
  yet.
- *The oversize check is `len(body) == maxMetadataBytes`.* A response that is
  exactly 16 MiB is rejected as oversized even though it arrived whole. This is
  a real false positive, judged acceptable because a metadata response that
  size is already a bug report. Decide whether you agree.
- *`unwrapURLError` strips one wrapper.* It returns `uerr.Err`, which is
  usually a `net.OpError` and carries no URL. If some transport error someday
  embeds the URL deeper, the credential rides out in the message.
- *`decode` trusts `status == "ok"`.* A server could answer ok with a payload
  belonging to a different endpoint. Every caller nil-checks its own field and
  reports a server bug; check that they all do.
- *Nothing logs the stream URL.* `StreamURL` returns a string carrying the
  credential. Grep for every use of it and satisfy yourself none reaches a log
  or an error.

**How to check it.**

```sh
go test ./internal/subsonic/... -v     # expect 46 PASS lines, subtests included
go test ./internal/subsonic/ -fuzz=FuzzDecode -fuzztime=60s
grep -rn 'StreamURL' --include='*.go' .        # every caller, by eye
```

---

## internal/mpv — 200 lines

**What it does now.** Spawns one mpv with fixed flags, waits for it to open a
unix socket, and sends every command over that socket — including the track
load, which carries the credential.

**What could go wrong.**

- *`Start` deletes the file at `Options.Socket` before spawning.* That path
  comes from `config.Paths.Socket()` in normal use, but the field is settable
  and this is an unconditional `os.Remove`. If anything ever passes a path a
  user cares about, shanty deletes it without asking.
- *Events are dropped when the channel is full.* The buffer is 64 and a slow
  reader loses events rather than blocking playback. A dropped `end-file` is a
  queue that does not advance — the album stops between tracks and nothing
  says why. Decide whether losing events is the right trade against blocking.
- *The environment is inherited whole.* mpv gets the user's full environment,
  because PipeWire, PulseAudio and ALSA all read from it. The reasoning is that
  shanty adds nothing of its own, so nothing of shanty's can leak — check that
  the reasoning holds by reading `Start`.
- *`Close` kills the process.* Three escalating steps, each unchecked. Satisfy
  yourself that a failure at any step still ends with a dead process.

**How to check it.**

```sh
go test ./internal/mpv/ -race -v       # expect 10 PASS
```

The hygiene claim is the one worth breaking yourself:

```sh
# put the credential where it must never be
sed -i 's|"--input-ipc-server="+opt.Socket,|"--input-ipc-server="+opt.Socket, "https://x/stream.view?t=deadbeefdeadbeefdeadbeefdeadbeef\&s=brackish",|' internal/mpv/mpv.go
go test ./internal/mpv/ -run ChildProcessHygiene    # expect FAIL
git checkout internal/mpv/mpv.go
```

---

## What the author did not verify

Labelled plainly, because a packet that claims more than it checked is the
failure it exists to prevent.

- **mpv has never run.** It is not installed on the machine this was written
  on. Every mpv test drives a stub that is the test binary re-executed. The
  flags, the protocol and the argv hygiene are all real; whether *actual* mpv
  accepts these flags and answers these commands is unverified until the
  integration job exists and a person plays a track.
- **No real server has been contacted.** Everything runs against the in-process
  fake. Whether Navidrome's responses match the shapes decoded here is
  unverified.
- **CI first ran on 2026-09-07** and passed all three jobs. Note that the
  `check` job skips `make standard`, because the standard lives outside this
  repository and is not published — a green check job is not evidence the
  conformance check passed.
- **The isolation suite does not exist yet**, so "writes are confined to four
  directories" is currently an argument from reading `Paths.All()`, not a test.
- **No release has been built**, so the binary-size budget has never had an
  artifact to measure.

## The reviewer's questions

Answer these in `docs/reviewed.md` or here, citing files:

1. Every path this code writes to, and what permits each.
2. Every way bytes enter the process, and the gate on each.
3. Every external process executed, and with what environment.
4. Every deletion path, and what happens when it is pointed somewhere wrong.
5. Which check proves each claim above.

## Off-card findings

What the read turned up that no card pointed at. "None" is a legal answer and a
tracked one.
