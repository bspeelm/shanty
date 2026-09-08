# Your first run

Run `shanty`. If it has not been configured yet, it asks you four questions,
tests the answers against your server, and writes its configuration files
itself. There is nothing to create by hand.

```
shanty setup — two files, in /home/you/.config/shanty

Server URL [https://music.example.org]: https://music.example.org
Username: you
API key (press enter to use a password instead):
Password:

checking… the server accepted it.

wrote /home/you/.config/shanty/config.toml
wrote /home/you/.config/shanty/credentials.toml (0600, holding the token and salt)

Your password was not saved. shanty stored the hash the Subsonic
protocol sends instead, which works against this server and nowhere else.

next: shanty
```

After that you are looking at the list of artists on your server. Use the arrow
keys to move, press `enter` to open an artist and then an album, and `enter`
again on a track to start playing it.

## What is on the screen

```
shanty · Aoi · Slipway
────────────────────────────────────────────────────────────────
  1. Harbour Light                                          3:41
> 2. Slipway                                                3:00
  3. Low Water                                              4:12
────────────────────────────────────────────────────────────────
▶ Slipway · Aoi                            1:23 / 3:00   vol 80%
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━───────────────────────────────────
↑↓ move · gg top · G end · enter open · esc back · / filter · :…
```

The top line names the screen you are on. Below it is the list, one row per
artist, album or track.

Under the second rule is the player. It shows whether the track is playing or
paused, its title and artist, how far into it you are, its length, and the
volume. The bar beneath fills from left to right as the track plays. A track
whose length your server did not report shows the elapsed time on its own and
an empty bar, because there is nothing to measure the position against.

Both lines appear only while something is playing. Before that the player says
`nothing playing` and there is no bar.

The last line lists the keys you are most likely to want. It is replaced by a
message when shanty has something to tell you, and by what you are typing when
you are filtering or entering a command.

## The four questions

**Server URL.** The address you use to reach your server in a browser,
including `https://`. If your server is behind a reverse proxy or is served
from a subdirectory, use the same address you would type into a browser.

**Username.** Your account name on that server.

**API key.** If your server supports API keys, create one in its web interface
and paste it here. An API key is a single string that identifies you, and you
can revoke it from the server at any time without changing your password. If
you do not have one, press enter to move on.

**Password.** Your account password. It is not displayed as you type, and it is
not saved to disk.

## What happens to your password

Subsonic servers do not require the password itself to sign in. They accept a
scrambled value derived from the password and a random string, and that pair is
what shanty writes to its configuration.

The practical consequence is that if someone copies your `credentials.toml`,
they can play music from that one server, but they cannot use the file to sign
in to your email, or to any other service where you used the same password.

If your server offers API keys, they are better still, because you can revoke
one from the server without changing anything else. `shanty doctor` tells you
when your server supports API keys and you are not using one.

## Nothing is saved if the credential does not work

shanty tries the credential against your server before writing either file. If
it is rejected, shanty says so, writes nothing, and you can run `shanty setup`
again. You will not end up with a configuration that looks correct but fails
later.

## Changing the configuration afterwards

Run `shanty setup` again to point shanty at a different server or to use a
different credential. It overwrites both files.

You can also edit the files by hand. They are plain TOML and shanty does not
mind which of you wrote them. The [Credentials](Credentials) page lists
everything that can go in them.

## Why setup requires a terminal

shanty will not read a password from a pipe or a redirected file. Anything
piped into a command tends to be recorded in a shell history file, which is not
a safe place for a password.
