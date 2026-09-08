# Credentials

Two files, never one.

| file | mode | holds |
|---|---|---|
| `~/.config/shanty/config.toml` | 0644 | the server URL and your username |
| `~/.config/shanty/credentials.toml` | 0600 | exactly the secret |

The split is so that `config.toml` can be pasted into a bug report and the
credential cannot.

## The four modes, in order

shanty uses the strongest one present. More than one may be there — migrating
from a password to an API key should not require getting the order right.

### 1. An API key

```toml
api_key = "..."
```

One revocable string, killed server-side in a click. Navidrome makes them in
its web interface. This is the one to use if your server offers it, and
`doctor` will say so when it does.

### 2. A token and salt

```toml
token = "..."
salt  = "..."
```

What the Subsonic protocol actually sends: an MD5 of your password with a
random salt. Replayable against this server and useless anywhere else, because
it is not the password. This is what `shanty setup` writes when you give it a
password.

### 3. A password file

```toml
password_file = "/run/agenix/navidrome"
```

A path to something your own tooling manages — an agenix secret, a `pass(1)`
entry through process substitution, whatever you already trust. shanty reads
it, trims the trailing newline every editor adds, and hashes it fresh for each
request.

Regular files must be 0600 or stricter. A pipe is not mode-checked, because a
pipe's permissions are not what protects it.

### 4. A plaintext password

```toml
password = "..."
```

shanty **reads** this if you wrote it, because refusing would only move you to
a client that will. It **never writes** it — the type `Save` marshals has no
password field at all, so that is a property of the code rather than a check
someone could forget.

`doctor` warns whenever it sees one, and names what to use instead.

## The permission check

If `credentials.toml` is readable by anyone else, shanty refuses to start and
prints the exact `chmod`. Refusing rather than warning is deliberate: a warning
is a thing people scroll past on the way to their music.

Stricter than 0600 is fine — 0400 is what an agenix secret arrives as.
