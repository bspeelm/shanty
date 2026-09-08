# Troubleshooting

Run `shanty doctor` first. Most of what follows is a longer version of
something it already told you.

## "mpv is not on PATH"

mpv is what plays the audio. [Installing](Installing) has the command for your
system.

On Fedora Silverblue, Kinoite or Bazzite, `dnf install` cannot change the
system. Use `rpm-ostree install mpv` and reboot. If that fails while resolving
kernel modules, `rpm-ostree update --install mpv` does the system update and
the install together, in one reboot.

## mpv is installed and shanty still cannot find it

They are probably not in the same place.

shanty talks to mpv through a socket file, so both have to see the same
filesystem. If shanty is running inside a container and mpv is on the host — or
the other way round — they cannot reach each other. Put both on the same side.

A **flatpak** mpv cannot work at all. Flatpak gives it a private view of the
system, so the socket shanty makes is not there as far as mpv is concerned.

## "the credentials file is readable by other accounts"

```sh
chmod 600 ~/.config/shanty/credentials.toml
```

shanty stops rather than warning, so this cannot be scrolled past.

## "the server answered with an HTML page rather than JSON"

Something is answering instead of your music server — a reverse proxy, a login
page, a captive portal. Open the address in a browser and see what comes back.

## "the server refused the credential"

The server is reachable and your credential is wrong. Check that `username` in
`config.toml` matches the account the credential belongs to, or run
`shanty setup` again.

If your account signs in through LDAP, the scrambled-password method may be
turned off for it. Use an API key instead.

## The music stops between tracks

If mpv has stopped, shanty says so on the status line rather than quietly
restarting it. Restart shanty.

If it keeps happening, try the same track in mpv directly and see what mpv
says.

## A key does nothing

Check which screen you are on. Back at the top level does nothing, so you
cannot leave the player by pressing back one time too many.

## A track name looks odd

Anything a terminal would act on as an instruction is removed from names before
they are drawn, so a name containing those characters appears without them.

## Removing it

```sh
shanty uninstall
```

Then delete the binary from wherever you installed it.
