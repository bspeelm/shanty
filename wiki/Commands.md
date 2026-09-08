# Commands

Five, and no more than five. Each one is in the README too, and a test holds
the two lists together.

### `shanty`

Browse and play. On a machine with nothing configured it runs the setup
questions first — see [Your first run](Your-first-run).

### `shanty setup`

Ask for a server and a credential again, and rewrite both files. Use it to
change server, or to move from a password to an API key.

Nothing is written unless the server accepts the credential.

### `shanty doctor`

Check the setup and say what is wrong. Every failing line carries the command
or the configuration change that fixes it — see [The doctor](The-doctor) for
what each check means.

```sh
shanty doctor        # for you
shanty doctor -json  # for a script
```

Exits non-zero if anything failed. Warnings do not fail it: they are things
worth knowing, not things that stop playback.

`doctor` creates nothing. A command that only reports should not leave a
directory behind as the price of having run.

### `shanty uninstall`

Remove every directory shanty made, and print which. It removes exactly the
four in [Where it puts things](Where-it-puts-things) and nothing else.

It refuses to delete through a symlink: a directory shanty removes is one a
link could point anywhere.

It does not remove the binary. That is wherever you put it — `~/.local/bin` if
you used the installer.

### `shanty version`

What this binary is. A development build says `dev` rather than claiming a
number nobody tagged.

### `shanty help`

The list.
