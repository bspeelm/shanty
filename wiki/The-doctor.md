# The doctor

`shanty doctor` asks eight questions, in the order that makes the answers
useful: the local setup first, because there is no point asking a server
anything until mpv and the configuration are in order.

Every failure and every warning carries a fix. A test asserts that, so a check
added later cannot arrive without one.

| check | what it means when it fails |
|---|---|
| `mpv` | mpv is not on PATH. The fix names the right command for your system — including `rpm-ostree` on an image-based one, where `dnf install` cannot write. |
| `mpv-version` | mpv would not report a version. Informational: shanty does not yet declare a minimum, and inventing one would be a gate no record supports. |
| `runtime-dir` | The directory the mpv socket lives in is unusable, or not 0700. It is reported, not created — `doctor` leaves nothing behind. |
| `config` | `config.toml` will not parse, or names no server. Warns instead of failing if the server is plain HTTP. |
| `credentials` | No credential, or one anybody on the machine can read. Warns if it is a plaintext password, and names the better options. |
| `server` | The server did not answer. Points at reachability, not at your password. |
| `auth` | The server answered and refused the credential. Points at `credentials.toml`, not at your firewall. |
| `auth-mode` | The server offers API keys and shanty is using something weaker. |

## Why `server` and `auth` are separate

A server that answers and then refuses the credential is not unreachable. If
those were one check, a wrong password would send you to look at your network.

## `-json`

```sh
shanty doctor -json
```

The same report as a machine-readable object: an `id`, a `severity` of
`fail`, `warn`, `pass` or `skip`, a `summary`, and a `fix` on anything that is
not passing.

Warnings do not make it exit non-zero. Only failures do.

## Skips

A check that could not be answered says so rather than guessing. If the
configuration is unusable there is nothing to ask a server, so `server`, `auth`
and `auth-mode` skip with a reason instead of failing three times for one
cause.
