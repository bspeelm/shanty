# Your first run

Type `shanty`. With nothing configured it asks four questions and writes the
files itself.

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

## What it did with your password

Nothing durable. The Subsonic protocol authenticates with an MD5 of the
password and a random salt, and that pair is what shanty writes. A stolen
`credentials.toml` replays against this one server and is useless anywhere
else — which matters because passwords get reused.

If your server offers **API keys**, use one instead. They are a single
revocable string, killed server-side in a click, and setup asks for one before
it asks for a password. `shanty doctor` will tell you when your server supports
them and you are not using one.

## Nothing is written if it does not work

setup checks the credential against the server before saving anything. A typo
is a question asked again, not a configuration that looks right and fails
later.

## Changing it later

`shanty setup` again. Or edit the two files by hand — they are plain TOML and
shanty is not precious about which of you wrote them. See
[Credentials](Credentials) for the four things `credentials.toml` can hold.

## It needs a terminal

setup will not read a credential from a pipe. Anything typed into a pipe lives
in whatever produced the pipe, which is usually a shell history.
