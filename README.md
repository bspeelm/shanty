<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/images/shanty-dark.png">
    <source media="(prefers-color-scheme: light)" srcset="docs/images/shanty-light.png">
    <img alt="shanty — a hut on the shore, drawn in ASCII" src="docs/images/shanty-light.png" width="820">
  </picture>
</p>

<h1 align="center">shanty</h1>

<p align="center"><em>a music player for your own server</em></p>

shanty is a terminal application for playing music from a Subsonic or Navidrome
server that you run yourself. It lists the artists on your server, then the
albums by an artist you choose, then the tracks on an album. When you select a
track, shanty plays it and reports the play back to your server, so your
listening history stays on the machine you control.

shanty does not store music on your computer, download files, or keep a local
library. The audio is decoded and played by [mpv](https://mpv.io), a separate
media player that shanty starts and controls, which means shanty can play any
format mpv supports.

## Requirements

You need mpv installed. shanty finds music on your server and controls
playback; mpv does the actual decoding and produces the sound. shanty will not
start without it, and `shanty doctor` will tell you the command to install it
on your system. The [Installing](https://github.com/bspeelm/shanty/wiki/Installing) page covers
installing mpv on each platform, including containers and image-based systems
such as Fedora Silverblue.

## Installing

Download and install the latest release:

```sh
curl -fsSL https://raw.githubusercontent.com/bspeelm/shanty/main/bootstrap/install.sh | sh
```

This puts the `shanty` binary in `~/.local/bin`. 

You can also build it from a clone of this repository:

```sh
make install-binary
```

## Using it

Run `shanty`. The first time you do, it asks for your server address, your
username, and either an API key or a password. It checks those against your
server and then writes its own configuration files, so there is nothing to
create by hand.

If you give it a password, that password is not saved. Subsonic servers accept
a scrambled form of a password instead of the password itself, and that is what
shanty stores. Someone who copies the file can play your music, but cannot use
it to sign in to anything else.

```sh
shanty              # browse and play
shanty setup        # change the server or the credential
shanty doctor       # check the setup and report what is wrong
shanty doctor -json # the same report, as JSON
shanty uninstall    # remove the directories shanty created
shanty version      # print the version of this build
shanty help         # list the commands
```

Use the arrow keys or `j` and `k` to move through a list, `enter` to open an
artist or album, and `enter` on a track to play it. Press `esc` to go back a
level. While a track is playing, `space` pauses and resumes, `n` and `p` move
between tracks, `[` and `]` seek backward and forward, `+` and `-` change the
volume, and `q` quits. The
[Keys](https://github.com/bspeelm/shanty/wiki/Keys) page lists all of them.

## How shanty handles your data

shanty connects to your server and to nothing else. It does not check for
updates, report errors, collect usage data, or contact any third party. Plays
are reported to your own server's endpoint. If you want them forwarded to
Last.fm or ListenBrainz, that is configured on the server you already run,
rather than by giving another service your credentials.

Your credential is stored in a file that only your user account can read.
shanty checks the file's permissions when it starts and refuses to run if
anyone else on the machine could read it, telling you the command that fixes
it. The credential is never passed to mpv as a command-line argument, because
command-line arguments are visible to every user on the system.

Certificate verification cannot be disabled. There is no option, flag, or
environment variable for it. If your server uses a self-signed certificate, add
that certificate to your system's trust store.

shanty writes to four directories, listed on the
[Where it puts things](https://github.com/bspeelm/shanty/wiki/Where-it-puts-things)
page, and `shanty uninstall` removes all of them. It does not read or modify
your mpv configuration.

The [Security](https://github.com/bspeelm/shanty/wiki/Security) page explains
all of this in more detail.

## Current state

Browsing and playback work. Searching, playlists, starring tracks, queueing
plays while offline, and resuming where you left off on another device are
planned next. Cover art and synchronised lyrics come after that.

The [wiki](https://github.com/bspeelm/shanty/wiki) is the documentation.
[CONTRIBUTING.md](CONTRIBUTING.md) describes how to build and test the project.

## Maintenance

shanty is written and maintained by one person for their own use, and will be
maintained for as long as that remains true. There is no team behind it and no
support commitment.

## Licence

MIT. See [`LICENSE`](LICENSE).
