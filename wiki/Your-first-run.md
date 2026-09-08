# Your first run

Run `shanty`. With nothing configured yet, it asks four questions and sets
itself up.

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

Then you are looking at your artists. Move with the arrow keys, press `enter`
to open one, `enter` again on a track to play it.

## Your password is not kept

Subsonic servers accept a scrambled form of your password rather than the
password itself, and that is what shanty stores. If someone takes the file they
can play your music; they cannot sign in anywhere else with it.

If your server offers **API keys**, use one. They are a single string you can
cancel from the server's web interface without changing your password. setup
asks for one before it asks for a password, and `shanty doctor` will tell you
when your server supports them and you are not using one.

## Nothing is written unless it works

setup tries the credential against your server before saving. A typo means the
question comes round again, not a broken setup you discover later.

## Changing it

Run `shanty setup` again to point at a different server or use a different
credential. Or edit the two files yourself — they are plain text, and
[Credentials](Credentials) describes what can go in them.

## It needs a real terminal

setup will not take a password from a pipe, because anything piped in tends to
end up in a shell history.
