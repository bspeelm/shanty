# Installing

shanty is one static binary. It needs **mpv**, because it does not decode audio
itself — mpv does that in its own process, and ADR-011 says why.

## The binary

From a release:

```sh
curl -fsSL https://raw.githubusercontent.com/bspeelm/shanty/main/bootstrap/install.sh | sh
```

It downloads the archive for your platform, checks it against the release's
`checksums.txt`, and installs to `~/.local/bin`. Add `--verify` to check
provenance as well; that needs the `gh` CLI.

The checksum proves the bytes match what the release published. The attestation
proves who published them, which a checksum cannot — whoever could swap the
archive could swap the checksums file beside it.

From a checkout, needing no release and no network:

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

**A flatpak mpv will not work.** shanty starts mpv and talks to it over a unix
socket, so both have to see the same filesystem at the same path. The flatpak
sandbox gives mpv a different `XDG_RUNTIME_DIR`, so the socket shanty creates
is not there under the name mpv was told to open. It fails looking like a
permissions problem. ADR-014 has the detail.

The same rule applies to containers: if you run shanty inside a Toolbx or
distrobox, mpv has to be inside it too. If you run shanty on the host, mpv goes
on the host.

## Checking it worked

```sh
shanty doctor
```

Every line either passes or names the command that fixes it.
