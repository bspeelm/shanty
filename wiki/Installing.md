# Installing

shanty is a single binary. It needs **mpv**, which is what actually plays the
audio — shanty finds the music and mpv makes the sound.

## The binary

```sh
curl -fsSL https://raw.githubusercontent.com/bspeelm/shanty/main/bootstrap/install.sh | sh
```

This downloads the build for your system, checks it against the checksums the
release publishes, and puts it in `~/.local/bin`.

Add `--verify` to also check who built it. That needs the `gh` command line
tool installed. The checksum tells you the file was not corrupted in transit;
the verification tells you it came from this project's build, which a checksum
cannot.

From a clone of the repository, with no release and no network:

```sh
make install-binary
```

## mpv

| system | command |
|---|---|
| Debian, Ubuntu | `apt install mpv` |
| Fedora, RHEL | `dnf install mpv` |
| Arch | `pacman -S mpv` |
| macOS | `brew install mpv` |
| Fedora Silverblue, Kinoite, Bazzite | `rpm-ostree install mpv`, then reboot |

### mpv has to be reachable from where shanty runs

shanty starts mpv and talks to it through a socket file, so both programs have
to see the same filesystem.

- **A flatpak mpv will not work.** Flatpak gives it a private view of the
  system, and it cannot see the socket shanty makes. The failure looks like a
  permissions error.
- **Inside a container**, install mpv in the same container you run shanty in.
  If shanty runs on the host, mpv goes on the host.

## Checking it

```sh
shanty doctor
```

Every line either passes or tells you the command to run.
