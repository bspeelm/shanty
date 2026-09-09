# Packaging

Two packages, made two different ways, and only one of them is a repository.

Fedora's rpm is built **from source** by Copr out of `shanty.spec`. Debian's
`.deb` is built by goreleaser at release time from the binary it has just
produced, and attached to the GitHub release as a file. macOS gets a Homebrew
cask from the same tag.

All three declare **mpv**. shanty decodes no audio itself, so a shanty without
mpv is a music player that cannot play music. `shanty doctor` could only ever
report that after the fact; a package manager can fix it before you notice.

## The version

`Version:` in `shanty.spec` is the one place a version is written. The tag is
derived from it, and the release workflow refuses a tag that disagrees:

```sh
v=$(sed -n 's/^Version:[[:space:]]*//p' packaging/shanty.spec)
test "v$v" = "$GITHUB_REF_NAME"
```

goreleaser stamps the binary and the deb from the tag; Copr reads the spec.
Those are two reads of what has to be one number, which is why the guard is
there.

## Cutting a release

```sh
make release VERSION=0.3.0     # tests, bumps the spec, opens the pull request
# merge it, then:
git switch main && git pull
make release-tag               # tags the merged bump
```

Two steps because `main` requires a pull request. That is not only a rule to
satisfy: Copr reads `Version:` from the spec **at the tagged commit**, so
tagging a commit that still carries the old number would publish an rpm under
it. The bump has to be on main before the tag exists.

The tag is the trigger for everything downstream — GitHub Actions builds the
archives, the deb and the cask; the Copr webhook builds the rpms. Branch rules
do not cover tags, so `release-tag` needs no pull request of its own.

`make release` refuses without a review packet at `docs/review/vX.Y.Z.md`,
because the release workflow refuses too and finding out after the tag is
worse.

## Copr

Two usernames: the Copr project is under the Fedora account **bspeelman**, the
source and releases under the GitHub name **bspeelm**.

One-time setup:

```sh
copr-cli create shanty \
    --chroot fedora-rawhide-x86_64 \
    --chroot fedora-44-x86_64 \
    --chroot fedora-43-x86_64 \
    --chroot fedora-44-aarch64 \
    --description "A terminal client for Navidrome and other Subsonic servers" \
    --instructions "dnf copr enable bspeelman/shanty && dnf install shanty"
```

Then add the package as an SCM build, cloning
`https://github.com/bspeelm/shanty`, spec `packaging/shanty.spec`, type `git`,
method `make_srpm`, with rebuild-on-webhook on.

### Making the webhook fire

Copr does not key off the GitHub event name. It reads `ref_type`, which is
present only in GitHub's **"Branch or tag creation"** event and never in
**Push**. Subscribing to Push silently never builds.

Copy the webhook URL from the Copr integrations page, **append `shanty/`**,
set the content type to `application/json`, and subscribe to *Branch or tag
creation* only. The trailing `/shanty/` makes any tag match.

| What you see | What it is |
|---|---|
| no delivery listed in GitHub at all | subscribed to Push |
| the delivery returns 415 | content type left as `x-www-form-urlencoded` |
| the delivery returns 404 | secret truncated, or the URL is missing `/shanty/` or its trailing slash |
| the delivery returns 200 and nothing builds | the package is not `scm`, rebuild-on-webhook is off, or the clone URL does not match |

`make copr` asks for a build by hand if the webhook misses one.

### Why the build is offline

Copr build roots have no network. `vendor/` is committed and the spec sets
`GOFLAGS=-mod=vendor` and `GOPROXY=off`, so a build that tried to reach out
would fail there rather than succeed by accident. CI regenerates `vendor/` on
every pull request and fails if it has drifted from `go.mod`.

## Debian and Ubuntu

The same tag produces `shanty_<version>_amd64.deb` and the arm64 one, attached
to the release and covered by `checksums.txt`.

```sh
sudo apt install ./shanty_0.2.0_amd64.deb
```

The leading `./` is not optional; without it apt looks for a package by that
name in your sources.

There is no apt source to add, no file in `sources.list.d`, and no signing key.
That is deliberate, and it has one consequence worth stating: **`apt upgrade`
will never bring you a new shanty.** A repository is something you have to keep
serving and re-signing for as long as anyone has it in their sources, and this
project does not promise that.

## macOS

```sh
brew install --cask bspeelm/shanty/shanty
```

The cask depends on the `mpv` formula, and strips the quarantine attribute on
install: the binary is unsigned, and Homebrew removed the flag that used to let
you opt out. ADR-012 owes the answer to signing it properly.

## Building either one without a release

```sh
make srpm                                      # an SRPM, the way Copr will
rpmbuild --rebuild --nodeps dist/shanty-*.src.rpm
goreleaser release --snapshot --clean --skip=publish   # archives, deb, cask
```

The rpm build runs `%check`, so a build that succeeds has also run the test
suite offline. On a machine without `dpkg`, look inside a deb with:

```sh
ar p dist/shanty_*_amd64.deb control.tar.gz | tar -xzO ./control
```
