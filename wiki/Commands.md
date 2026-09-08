# Commands

### `shanty`

Browse and play. On a machine that has not been set up yet, it asks the setup
questions first — see [Your first run](Your-first-run).

### `shanty setup`

Ask for a server and a credential, and write the two configuration files.

Use it to point at a different server, or to switch from a password to an API
key. Nothing is written unless the server accepts what you gave it.

### `shanty doctor`

Check the setup and report what is wrong. Every problem comes with the command
or the change that fixes it. [The doctor](The-doctor) explains each check.

```sh
shanty doctor        # readable
shanty doctor -json  # for a script
```

It exits with an error if a check failed. Warnings do not — those are things
worth knowing that will not stop you playing music.

`doctor` only looks. It does not create or change anything.

### `shanty uninstall`

Remove the directories shanty made, and print which ones. See
[Where it puts things](Where-it-puts-things) for the list.

It will not follow a symbolic link out of its own directories.

It does not remove the binary itself — that is wherever you installed it,
usually `~/.local/bin/shanty`.

### `shanty version`

Print the version. A build made from a clone rather than a release says `dev`.

### `shanty help`

List the commands.
