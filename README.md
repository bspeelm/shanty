<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/images/shanty-dark.png">
    <source media="(prefers-color-scheme: light)" srcset="docs/images/shanty-light.png">
    <img alt="shanty — a hut on the shore, drawn in ASCII" src="docs/images/shanty-light.png" width="820">
  </picture>
</p>

<h1 align="center">shanty</h1>

<p align="center"><em>a music player for your own server</em></p>

shanty plays the music on your own Subsonic or Navidrome server, from a
terminal. It shows your artists, then their albums, then the tracks. You pick
one and it plays, and when it finishes shanty tells the server, so your
listening history stays on the machine you run.

It does not keep a local library, download anything, or manage files. The music
stays where it is. Playback is handed to [mpv](https://mpv.io), so shanty plays
whatever mpv plays.

## Installing

shanty needs mpv installed — that is the part that makes the sound.

```sh
curl -fsSL https://raw.githubusercontent.com/bspeelm/shanty/main/bootstrap/install.sh | sh
```

Or, from a clone:

```sh
make install-binary
```

Full instructions, including containers and image-based systems, are in
[Installing](https://github.com/bspeelm/shanty/wiki/Installing).

## Using it

Run `shanty`. The first time, it asks for your server, your username and a
password or API key, checks them, and writes its configuration itself. Your
password is not stored — shanty keeps the scrambled form Subsonic servers
accept, which is no use anywhere else.

```sh
shanty              # browse and play
shanty setup        # change server or credential
shanty doctor       # check the setup and say what to fix
shanty doctor -json # the same, for a script
shanty uninstall    # remove everything shanty made
shanty version      # what this build is
shanty help         # the list
```

Arrow keys or `j` `k` to move, `enter` to open, `esc` to go back. `space`
pauses, `n` and `p` skip, `[` and `]` seek, `+` and `-` change volume, `q`
quits. The rest are in [Keys](https://github.com/bspeelm/shanty/wiki/Keys).

## What it will and will not do

It connects to your server and nothing else. No update checks, no telemetry, no
third-party services. Scrobbles go to your server's own endpoint — forwarding
them to Last.fm or ListenBrainz is a setting on the server you already run,
rather than another password on every machine.

It keeps your credential out of reach of other accounts on the same computer,
and refuses to start if the file holding it can be read by anyone else.

It cannot be told to skip certificate checking. If you use a self-signed
certificate, add it to your system's trust store.

It writes to four directories, leaves your mpv configuration alone, and removes
all four when you run `shanty uninstall`.

[Security](https://github.com/bspeelm/shanty/wiki/Security) covers this in
full.

## Status

Browsing and playback work. Search, playlists, starring, offline scrobbling and
resuming across devices are next; cover art and lyrics after that.

The **[wiki](https://github.com/bspeelm/shanty/wiki)** is the documentation.
[CONTRIBUTING.md](CONTRIBUTING.md) is for working on it.

## Maintenance

Written and maintained by one person, for their own use, and maintained while
that stays true. There is no team behind this and no support commitment.

## Licence

MIT — [`LICENSE`](LICENSE).
