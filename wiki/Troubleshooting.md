# Troubleshooting

Run `shanty doctor` first. It examines your setup and names the fix for
anything that is wrong, and most of the sections below are a longer explanation
of something it will already have told you.

## "mpv is not on PATH"

shanty does not decode audio itself; mpv does. The
[Installing](Installing) page lists the install command for each system.

On Fedora Silverblue, Kinoite or Bazzite, `dnf install` cannot modify the
operating system. Use `sudo rpm-ostree install mpv` and then reboot.

If that fails while resolving kernel modules, it is because your installed
kernel is older than the one the repositories now offer, and a layered package
cannot be rebuilt against it. Running `sudo rpm-ostree update --install mpv`
updates the system and installs mpv in a single operation, which resolves it
with one reboot instead of two.

## mpv is installed but shanty still cannot find it

The two are probably not installed in the same place.

shanty controls mpv through a socket file on disk, so both programs must be
able to see that file at the same path. If shanty is running inside a container
and mpv is installed on the host system, or the other way around, they cannot
reach each other.

Install mpv wherever you run shanty: inside the container if shanty runs in a
container, or on the host if it runs on the host.

An mpv installed as a **flatpak** will not work at all. Flatpak gives an
application its own private view of the filesystem, so it cannot see the socket
file shanty creates. shanty starts mpv successfully and then cannot communicate
with it.

## "the credentials file is readable by other accounts"

```sh
chmod 600 ~/.config/shanty/credentials.toml
```

shanty refuses to start rather than warning, so that this cannot be missed.

## "the server answered with an HTML page rather than JSON"

Something other than your music server is responding to shanty's requests. This
is usually a reverse proxy returning an error page, a login page, or a captive
portal on the network you are connected to.

Open the same address in a browser and see what comes back.

## "the server refused the credential"

Your server is reachable and responding, but it did not accept your credential.

Check that the `username` in `config.toml` is the account the credential
belongs to, or run `shanty setup` again to enter it afresh.

If your account signs in through LDAP, your server may not accept the scrambled
password that Subsonic clients normally send. Create an API key in the server's
web interface and use that instead.

## Playback stops between tracks

If mpv has stopped running, shanty reports that on the status line rather than
restarting it silently. Quit and start shanty again.

If it keeps happening, play the same track with mpv directly and see what mpv
reports.

## A key does nothing

Check which screen you are on. Pressing back on the artist list does nothing,
because there is no screen above it.

## A track or artist name looks wrong

Characters that a terminal would interpret as an instruction are removed from
names before they are displayed, so a name containing them appears without
them. See [Security](Security) for why.

## Removing shanty

```sh
shanty uninstall
```

Then delete the binary from wherever you installed it, usually
`~/.local/bin/shanty`.
