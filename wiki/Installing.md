# Installing

Installing shanty takes two steps: install the shanty binary, and install mpv.

shanty is a single executable file with no other dependencies. mpv is a media
player that must be installed separately. shanty handles finding music on your
server and controlling playback, while mpv decodes the audio and sends it to
your sound card. shanty will not start without mpv.

## Installing the shanty binary

The simplest way is to download the latest release:

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
