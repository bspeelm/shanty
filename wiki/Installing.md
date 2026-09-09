# Installing

Installing shanty takes two steps: install the shanty binary, and install mpv.

shanty is a single executable file with no other dependencies. mpv is a media
player that must be installed separately. shanty handles finding music on your
server and controlling playback, while mpv decodes the audio and sends it to
your sound card. shanty will not start without mpv.

## From your package manager

The best way, where there is one: your package manager installs mpv with
shanty, and shanty does not play anything without it.

| | |
|---|---|
| **Fedora** | `sudo dnf copr enable bspeelman/shanty && sudo dnf install shanty` |
| **Debian, Ubuntu, Mint** | download the `.deb` from the [latest release](https://github.com/bspeelm/shanty/releases/latest), then `sudo apt install ./shanty_*.deb` |
| **macOS** | `brew install --cask bspeelm/shanty/shanty` |
| **you already have Go** | `go install github.com/bspeelm/shanty/cmd/shanty@latest` |

Two of those need a word of explanation.

The `.deb` is a file on the release page, not a repository you add. There is no
apt source and no signing key, so **`apt upgrade` will never bring you a new
shanty** — you come back for the next one. A repository is something that has
to be served and re-signed for as long as anyone has it in their sources, and
this project does not promise that.

`go install` builds from source and installs nothing else, so mpv is yours to
install. `shanty doctor` names the command for your system.

These also place the shell completions, so pressing tab after `shanty` offers
its commands. Nothing is added to your shell's startup files.

## Installing the shanty binary yourself

On an image-based system, where `dnf` means `rpm-ostree` and a reboot, or
anywhere you would rather not involve a package manager:

```sh
curl -fsSL https://raw.githubusercontent.com/bspeelm/shanty/main/bootstrap/install.sh | sh
```

The script detects your operating system and processor architecture, downloads
the matching build, checks it against the checksums published with the release,
and copies it to `~/.local/bin/shanty`. If `~/.local/bin` is not on your `PATH`,
the script tells you and shows the line to add to your shell configuration.

To also confirm that the file was built by this project's release process and
not substituted afterwards, add `--verify`:

```sh
curl -fsSL https://raw.githubusercontent.com/bspeelm/shanty/main/bootstrap/install.sh | sh -s -- --verify
```

This requires GitHub's `gh` command-line tool to be installed. The checksum
check that always runs confirms the download was not corrupted or truncated.
The `--verify` check confirms who produced the file, which a checksum alone
cannot do, because anyone able to replace the download could replace the
checksum file alongside it.

If you have cloned the repository, you can build and install it directly
instead. This needs Go and no network access:

```sh
make install-binary
```

## Installing mpv

| Operating system | Command |
|---|---|
| Debian, Ubuntu | `sudo apt install mpv` |
| Fedora, RHEL, CentOS | `sudo dnf install mpv` |
| Arch Linux | `sudo pacman -S mpv` |
| macOS with Homebrew | `brew install mpv` |
| Fedora Silverblue, Kinoite, Bazzite | `sudo rpm-ostree install mpv`, then reboot |

### mpv must be installed where shanty runs

shanty controls mpv by writing to a socket file, which is a special file on
disk. Both programs must be able to see that file at the same path, which means
they must be installed in the same place as each other.

**Do not install mpv as a flatpak.** Flatpak deliberately gives an application
its own private view of the filesystem, so an mpv installed that way cannot see
the socket file shanty creates. shanty will start mpv successfully and then
fail to communicate with it, and the error looks like a permissions problem.

**If you run shanty inside a container**, such as a Toolbx or distrobox
container, install mpv inside that same container. If you run shanty directly
on your host system, install mpv on the host.

## Confirming it worked

```sh
shanty doctor
```

This reports on each part of your setup. Anything that is not working is listed
with the command or configuration change that fixes it. The
[The doctor](The-doctor) page explains what each check looks at.
