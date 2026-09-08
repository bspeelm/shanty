# Credentials

shanty keeps its settings in two files:

| file | permissions | contents |
|---|---|---|
| `~/.config/shanty/config.toml` | `0644` | your server address and username |
| `~/.config/shanty/credentials.toml` | `0600` | the secret, and nothing else |

They are separate so you can share or paste the first when asking for help
without the second going with it.

`shanty setup` writes both. You can also edit them by hand — they are plain
TOML.

## The four ways to sign in

shanty uses the strongest one it finds. Having more than one in the file is
fine; it does not matter what order they appear in.

### An API key — best

```toml
api_key = "..."
```

A single string, created in your server's web interface, that you can cancel
there without changing your password. Navidrome supports these.

`shanty doctor` tells you when your server offers them and you are using
something else.

### A token and salt

```toml
token = "..."
salt  = "..."
```

The scrambled form of your password that Subsonic servers actually accept. It
works against that one server and is no use anywhere else. This is what
`shanty setup` writes when you give it a password.

### A password file

```toml
password_file = "/run/agenix/navidrome"
```

A path to a file your own tooling manages — a secret from `agenix`, an entry
from `pass`, anything you already trust to hold it. shanty reads it when it
needs to sign in and does not copy it anywhere.

If the path is a normal file it has to be readable only by you. A pipe is not
checked, since its permissions are not what protects it.

### A plain password — worst

```toml
password = "..."
```

shanty will use this if you wrote it, but it will never write it for you. If
this file is taken, so is any other account where you used the same password.

`shanty doctor` warns whenever it sees one and says what to use instead.

## The permission check

If `credentials.toml` can be read by anyone else on the machine, shanty stops
and prints the command to fix it:

```
chmod 600 /home/you/.config/shanty/credentials.toml
```

Stricter is fine. `0400` is common for secrets managed by other tools.
