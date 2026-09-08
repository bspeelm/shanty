# Troubleshooting

Start with `shanty doctor`. Most of what follows is a longer version of
something it already told you.

## "mpv is not on PATH"

shanty does not decode audio; mpv does. See [Installing](Installing) for the
command on your system.

On an image-based Fedora — Silverblue, Kinoite, Bazzite — `dnf install` cannot
write `/usr`. Use `rpm-ostree install mpv` and reboot. If that fails to resolve
because of layered kernel modules, `rpm-ostree update --install mpv` does the
kernel and the package in one transaction and one reboot.

## mpv is installed and shanty still cannot find it

You are probably running them in different containers.

shanty starts mpv and talks to it over a unix socket, so both have to see the
same filesystem. If shanty runs inside a Toolbx and mpv is on the host, or the
other way round, they cannot meet. Put both on the same side.

A **flatpak** mpv cannot work at all: the sandbox gives it a different
`XDG_RUNTIME_DIR`, so the socket shanty creates is not there under the name mpv
was told to open.

## "the credentials file is readable by other accounts"

```sh
chmod 600 ~/.config/shanty/credentials.toml
```

shanty refuses to start rather than warning, because a warning is a thing
people scroll past.

## "the server answered with an HTML page rather than JSON"

Something is in front of your server — a reverse proxy, a login page, a captive
portal. Open the URL in a browser and see what answers. A Subsonic server
speaking to shanty answers JSON.

## "the server refused the credential"

The server is reachable; the credential is wrong. Check `username` in
`config.toml` against what is in `credentials.toml`, or run `shanty setup`
again.

If your account authenticates through LDAP, token authentication may be
disabled for it — the server says so with error 41, and an API key is the way
round it.

## Playback stops between tracks

mpv is told to open the next track early. If it stops instead, mpv may have
exited: shanty says so on the status line rather than silently restarting it,
because a restart loop turns "mpv is broken" into "the machine is slow".

Restart shanty. If it happens repeatedly, run `mpv <stream-url>` by hand and
see what mpv says.

## Nothing happens when I press a key

Check you are on the screen you think you are. `esc` at the top level does
nothing on purpose — leaving a music player by pressing back one time too many
is a bad surprise.

## It looks wrong in my terminal

shanty draws with bold, faint and reverse only, which every emulator supports.
If a track title looks strange, note that anything a terminal would obey is
stripped out of what the server sends before it is drawn — a title that looked
like an escape sequence will render as its printable characters.

## Removing it

```sh
shanty uninstall
```

Then delete the binary, wherever you put it.
