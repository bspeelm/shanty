# Credentials

shanty stores its configuration in two files:

| File | Permissions | Contents |
|---|---|---|
| `~/.config/shanty/config.toml` | `0644` | Your server's address and your username |
| `~/.config/shanty/credentials.toml` | `0600` | The credential, and nothing else |

They are kept apart so that you can show someone your `config.toml` when asking
for help without the credential being in it.

`shanty setup` creates both files for you. You can also write them yourself;
they are plain TOML.

## The four kinds of credential

`credentials.toml` can hold any one of four things. If more than one is
present, shanty uses the strongest, and the order they appear in the file does
not matter.

### An API key

```toml
api_key = "..."
```

An API key is a single string, created in your server's web interface, that
identifies you in place of a password. It is the best option when your server
supports it, because you can revoke a key from the server at any time without
changing your password or affecting anything else that uses it. Navidrome
supports API keys.

`shanty doctor` tells you when your server supports API keys and you are using
something else.

### A token and salt

```toml
token = "..."
salt  = "..."
```

Subsonic servers accept a scrambled value derived from your password, together
with the random string used to scramble it, instead of the password. This pair
works against that one server and is of no use anywhere else.

This is what `shanty setup` writes when you give it a password rather than an
API key.

### A password file

```toml
password_file = "/run/agenix/navidrome"
```

The path to a file holding your password, managed by something else you already
trust, such as `agenix` or `pass`. shanty reads the file when it needs to
authenticate and does not copy the contents anywhere.

If the path points at an ordinary file, that file must not be readable by other
users. If it points at a pipe, shanty does not check permissions, because a
pipe's permissions are not what protects it.

### A password

```toml
password = "..."
```

Your password, stored as text. shanty will use one if you write it, but it will
never write one itself.

This is the weakest option, because anyone who copies the file has your actual
password, and can try it on every other account where you used the same one.
`shanty doctor` warns whenever it finds one and tells you what to use instead.

## The permissions check

Every time it starts, shanty checks that `credentials.toml` cannot be read by
other users of the machine. If it can, shanty stops and prints the command to
fix it:

```
chmod 600 /home/you/.config/shanty/credentials.toml
```

Permissions stricter than `0600` are accepted. Files created by secret managers
are often `0400`, which is fine.
