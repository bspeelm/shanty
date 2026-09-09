# The doctor

`shanty doctor` examines your setup and reports on eleven things. Anything that
is not working is listed with the command or configuration change that fixes
it.

```
✓ mpv            mpv is at /usr/bin/mpv
✓ mpv-version    mpv 0.38.0
✓ runtime-dir    /run/user/1000/shanty will be made at 0700 when a track first plays
✓ config         https://music.example.org
! credentials    the credential is a plaintext password
  a stolen config yields whatever else that password opens
  fix: replace it with an api_key, or with password_file pointing at a secret you manage
✓ server         https://music.example.org answered
✓ auth           the credential works
! auth-mode      this server offers API keys and shanty is not using one
  an API key is revoked server-side in one click; a token is not
  fix: create one in the server's web interface and put it in credentials.toml as api_key
```

## What each check examines

**`mpv`** — whether the mpv media player is installed and can be found on your
`PATH`. shanty cannot play anything without it. If it is missing, the fix names
the right install command for your system, including image-based systems where
the usual package manager cannot modify the operating system.

**`mpv-version`** — which version of mpv you have, and whether it is new
enough. shanty needs **0.24.0** or newer: it always starts mpv with
`--prefetch-playlist`, so that albums play without a gap, and that option
arrived in 0.24.0. An older mpv is reported as a failure naming the number. A
version shanty cannot read is a warning rather than a refusal.

**`runtime-dir`** — the directory that will hold shanty's connection to mpv
while music is playing. The check confirms the directory is usable and readable
only by you. If it does not exist yet, that is normal; shanty creates it when
you first play something.

**`session`** — whether a session left by `:headless` is playing in the
background, and whether an mpv is running that no session owns. A background
process is the one thing here that cannot report itself, so this is where it is
named. Both of the states that are wrong carry the command that ends them.

**`config`** — whether `config.toml` can be read and names a server address
that shanty can use. This check produces a warning rather than an error if your
server address uses `http://` instead of `https://`, because your credential
and everything you play then cross the network unencrypted.

**`cover-art`** — how this terminal is sent pictures, and the value of `TERM`
it decided that from. kitty, Ghostty, WezTerm and iTerm2 draw album art;
everything else gets none, and no cover is fetched for it. It is a warning
rather than a failure, because a terminal that shows no pictures is a perfectly
good terminal.

The check exists because the answer is otherwise invisible. A terminal shanty
does not recognise shows no cover, and so does an album whose server has none,
and the two look exactly the same.

**`keys`** — whether the key bindings in `config.toml` will work. An action
that does not exist, one bound to no keys, and a key two actions both want are
each reported, with the list of everything that can be bound. shanty starts
either way and says so on the last row; a typo is not a reason to withhold your
music.

**`credentials`** — whether a credential exists, and whether the file holding it
can be read by other users of the machine. It warns if the credential is a
plain password and names the better options.

**`server`** — whether your server responds at all.

**`auth`** — whether your server accepts your credential.

**`auth-mode`** — whether your server supports a stronger kind of credential
than the one you are using. It warns if the server offers API keys and shanty
is authenticating with something else.

## Reading the report

| Symbol | Meaning |
|---|---|
| `✓` | Working |
| `!` | Working, but something is worth changing |
| `✗` | Broken, with the fix on the following line |
| `–` | Not checked, because an earlier failure made the answer meaningless |

Only `✗` makes the command exit with an error. Warnings do not, because they
describe things that will not stop you playing music.

`server` and `auth` are separate checks. A server that responds and then
rejects your credential is not unreachable: `server` passes, `auth` fails, and
the problem is your credential rather than your network.

## JSON output

```sh
shanty doctor -json
```

Each check is reported as an object with an `id`, a `severity` of `fail`,
`warn`, `pass` or `skip`, a `summary`, and a `fix` where one applies.
