# Installing

**shanty needs mpv, and everything else it needs it brings.** It plays nothing
on its own: every track is handed to mpv over a socket, which is why the
credential in a stream URL never reaches a command line
([ADR-011](https://github.com/bspeelm/shanty/blob/main/docs/decisions.md#adr-011--playback-uses-mpv-in-a-separate-process)).

Installed from a package manager, mpv comes with it and there is nothing else
to do. Installed any other way, mpv is yours to install, and `shanty doctor`
names the command for your system.

Whatever you install, shanty keeps its own things in four directories under
your home and writes nowhere else. It needs no root to run, and
`shanty uninstall` removes what it wrote and says what it left.

## What you need first

| | |
|---|---|
| **[mpv](https://mpv.io)** | Required, version 0.24.0 or newer. Every package below installs it for you. See [mpv has to live where shanty lives](#mpv-has-to-live-where-shanty-lives) — it is the one thing people get wrong. |
| **a Subsonic server** | Navidrome is the one shanty is tested against. Others speak the same protocol and are untested ([ADR-007](https://github.com/bspeelm/shanty/blob/main/docs/decisions.md#adr-007--navidrome-is-the-supported-server-others-are-untested)). |
| **curl** or **wget** | Only for the install script, and only one of them. |
| **Go** and **make** | Only if you build from source, which nobody has to. |

## The short version

Reach for your package manager:

```sh
sudo dnf copr enable bspeelman/shanty && sudo dnf install shanty
```

Then `shanty`. The first run asks for your server address, your username and a
credential, checks them against the server, and writes its own configuration.
There is nothing to create by hand.

## Every channel

| | for | |
|---|---|---|
| **dnf** | Fedora Workstation | `sudo dnf copr enable bspeelman/shanty && sudo dnf install shanty` |
| **apt** | Debian, Ubuntu, Mint | download the `.deb`, then `sudo apt install ./shanty_*.deb` |
| **Homebrew** | macOS, or Linux if you already use brew | `brew install --cask bspeelm/shanty/shanty` |
| **script** | image-based systems, or anywhere without the above | `curl -fsSL .../install.sh \| sh` |
| **Go** | people who already have Go | `go install github.com/bspeelm/shanty/cmd/shanty@latest` |
| **source** | contributors | `git clone`, then `make install-binary` |

**Reach for your package manager first.** The first three install mpv with
shanty, place the shell completions, and upgrade the way everything else on
your machine upgrades.

One exception, and it is worth knowing before you rely on it: **the `.deb` is a
file, not a repository.** There is no apt source to add and no signing key, so
`apt upgrade` will never bring you the next shanty — you come back for it. A
repository has to be served and re-signed for as long as anyone has it in their
sources, and this project does not promise that.

**The script earns its place on an image-based host** — Silverblue, Kinoite,
Bazzite — where `dnf` means `rpm-ostree` and a reboot for a binary that runs
perfectly well from `~/.local/bin`:

```sh
curl -fsSL https://raw.githubusercontent.com/bspeelm/shanty/main/bootstrap/install.sh | sh
```

The cost, stated plainly: the script is fetched over HTTPS and run **unsigned**,
before shanty exists to verify anything. No signature on a later artifact fixes
that. It is the same trade as any `curl | sh`.

**`go install` installs nothing but shanty.** No mpv, no completions. Run
`shanty doctor` afterwards and it will tell you what is missing.

## mpv has to live where shanty lives

shanty controls mpv through a socket, which is a file on disk. Both programs
have to see that file at the same path, so they have to be installed on the
same side of any boundary between them
([ADR-014](https://github.com/bspeelm/shanty/blob/main/docs/decisions.md#adr-014--mpv-must-run-in-the-same-filesystem-namespace-as-shanty)).

**Do not install mpv as a flatpak.** A flatpak deliberately gets its own view
of the filesystem, so it cannot see the socket shanty creates. shanty starts it
successfully and then cannot talk to it, and the failure looks like a
permissions problem rather than what it is.

**If you run shanty in a container** — Toolbx, distrobox — install mpv in that
same container. If shanty runs on the host, mpv goes on the host.

Installing shanty from dnf, apt or Homebrew gets this right by construction:
the package manager installs both in the same place.

If you need mpv on its own:

| | |
|---|---|
| Debian, Ubuntu | `sudo apt install mpv` |
| Fedora, RHEL, CentOS | `sudo dnf install mpv` |
| Arch Linux | `sudo pacman -S mpv` |
| macOS with Homebrew | `brew install mpv` |
| Silverblue, Kinoite, Bazzite | `sudo rpm-ostree install mpv`, then reboot |

## Checking what you got

Every download is published with `checksums.txt`, and the install script checks
against it always. That proves the bytes are the ones the release published,
and not that the release published them:

```sh
sha256sum -c checksums.txt --ignore-missing
```

To prove **who** produced the file, which a checksum cannot — anyone able to
replace a download could replace the checksum beside it — the release also
publishes a Sigstore attestation:

```sh
gh attestation verify shanty_linux_amd64.tar.gz \
    --repo bspeelm/shanty --bundle attestation.jsonl
```

The install script does this too, with `--verify`, and needs `gh` installed for
it:

```sh
curl -fsSL https://raw.githubusercontent.com/bspeelm/shanty/main/bootstrap/install.sh | sh -s -- --verify
```

Without `--verify` it says which check it did not perform rather than implying
it did.

## macOS: Gatekeeper

The macOS binary is **not signed** by an Apple developer account. The Homebrew
cask strips the quarantine attribute on install, because Homebrew removed the
flag that used to let you opt out — so `brew install --cask` works.

Downloading the tarball by hand does not, and macOS will refuse to run it.
Signing it properly is an open question
([ADR-012](https://github.com/bspeelm/shanty/blob/main/docs/decisions.md#adr-012--macos-support-is-not-yet-described-status-open)).

## What gets installed

From dnf or apt:

| | |
|---|---|
| `/usr/bin/shanty` | the binary |
| `/usr/share/bash-completion/completions/shanty` | and the zsh and fish equivalents |
| `/usr/share/doc/shanty/` | the licence and the README |

From Homebrew, under whatever `brew --prefix` reports:

| | |
|---|---|
| `bin/shanty` | the binary |
| Homebrew's own completion directories | one file each for bash, zsh and fish |

From the script, or `make install-binary`:

| | |
|---|---|
| `~/.local/bin/shanty` | the binary |
| `~/.local/share/bash-completion/completions/shanty` | and `~/.config/fish/completions/shanty.fish` |

Either way, shanty's own files go in the four directories on the
[Where it puts things](Where-it-puts-things) page, and nowhere else.

## Tab completion

Every route but `go install` sets this up. Press tab after `shanty` and it
offers the commands rather than listing the directory you are standing in.

Nothing is added to `~/.bashrc` or any other startup file: each shell reads
completions from a directory of its own, and the file goes there. Open a new
terminal afterwards.

dnf, apt and Homebrew place all three, for bash, zsh and fish. The script and
`make install-binary` place bash and fish.

If you installed with `go install`, or want to place them yourself:

```sh
shanty completions install
```

That writes the bash and fish files, for whichever of the two shells is on your
machine. zsh reads completions only from a root-owned directory, so on Linux
that one is placed by hand:

```sh
shanty completions zsh | sudo tee /usr/share/zsh/site-functions/_shanty
```

## Confirming it worked

```sh
shanty doctor
```

It reports on eleven things, from whether mpv is installed and new enough to
whether your server accepts your credential. Anything wrong is listed with the
command or the configuration change that fixes it. The
[The doctor](The-doctor) page explains each check.

## Removing it

From a package manager, remove it the way you remove anything else —
`sudo dnf remove shanty`, `sudo apt remove shanty`,
`brew uninstall --cask shanty`.

Either way, this removes what shanty itself wrote:

```sh
shanty uninstall
```

It deletes the four directories, and the completion files if the script or
`make install-binary` placed them. It names everything it removed and
everything that was not there, and it refuses while a session left by
`:headless` is still playing.

It does not delete the binary. A package manager owns that one; the script's
copy is wherever you put it, usually `~/.local/bin/shanty`.
