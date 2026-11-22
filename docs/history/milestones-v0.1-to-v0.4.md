# The milestones of v0.1 to v0.4

This was §12 of `PLAN.md` until the four milestones in it had shipped or closed.
It is kept as it was written.

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

Three things shipped in it that this list did not plan for. `:headless` closes
the interface and leaves the music playing, commanded from any shell, which
ADR-018 records as a session rather than the daemon §3 refuses. `:wiki`
explains the commands without leaving shanty. And the packages moved here from
v0.4: dnf, apt and Homebrew each install shanty and mpv together, and CI builds
and installs every one of them.

**v0.3 — the comforts.** Cover art, using the kitty graphics protocol and the
iTerm2 inline image protocol behind one interface, with a text fallback always.
Sixel is in the backlog rather than shipped: it needs a palette quantiser and a
band encoder, which the code budget does not hold.

Synchronised lyrics, palette themes and MPRIS were in this milestone and are
now in the backlog. Each is wanted by nobody who has said so, and a milestone
is for what is being built rather than for everything that would be pleasant.
ADR-021 records the code budget this leaves.

**v0.4 — distribution. Closed with nothing built.** v0.2 shipped the packages,
and ADR-024 refuses the four things this milestone was left holding: signing and
notarising the macOS build, a manual page, an Arch package, and a job that
installs from the Homebrew tap. Each refusal names what would bring the work
back. The routes that exist are the distribution: dnf, apt, Homebrew, and the
install script.
