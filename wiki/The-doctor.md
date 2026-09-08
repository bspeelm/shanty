# The doctor

`shanty doctor` runs eight checks and reports on each. Anything that is not
passing comes with the command or the change that fixes it.

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

## The checks

| | what it is looking at |
|---|---|
| `mpv` | whether mpv is installed and reachable |
| `mpv-version` | which mpv, reported for your information |
| `runtime-dir` | the directory the connection to mpv will live in |
| `config` | that `config.toml` reads and names a server |
| `credentials` | that a credential exists and only you can read it |
| `server` | whether the server answers |
| `auth` | whether it accepts your credential |
| `auth-mode` | whether a better kind of credential is available |

## Reading the results

**✓** fine. **!** works, but you would probably want to change it. **✗**
broken, and the fix is on the next line. **–** not checked, because something
earlier made the answer meaningless.

Only ✗ makes `doctor` exit with an error.

`server` and `auth` are separate. A server that answers and then refuses your
credential is not unreachable — `server` passes and `auth` fails, so the
problem is your password rather than your network.

## As data

```sh
shanty doctor -json
```

Each check comes back with an `id`, a `severity` of `fail`, `warn`, `pass` or
`skip`, a `summary`, and a `fix` where there is one.
