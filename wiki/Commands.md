# Commands

shanty has six commands. Running `shanty` with no arguments browses and plays;
the rest are named.

## `shanty`

Opens the browser and plays music. This is the command you will use almost all
of the time.

If shanty has not been configured yet, it runs the setup questions first. See
[Your first run](Your-first-run).

## `shanty setup`

Asks for a server address, a username, and a credential, then writes shanty's
two configuration files.

Use it when you want to point shanty at a different server, or to replace a
password with an API key. It overwrites both files, and it writes nothing
unless your server accepts the credential you gave it.

## `shanty doctor`

Examines your setup and reports on eight things, from whether mpv is installed
to whether your server accepts your credential. Anything that is wrong is
listed together with the command or configuration change that fixes it. The
[The doctor](The-doctor) page explains each check.

```sh
shanty doctor        # human-readable report
shanty doctor -json  # the same report as JSON, for scripts
```

The command exits with a non-zero status if any check failed. Warnings do not
cause a non-zero exit, because they describe things you would probably want to
change rather than things that stop shanty working.

`shanty doctor` only inspects. It does not create, change, or delete anything.

## `shanty uninstall`

Deletes the directories shanty created and prints which ones it removed and
which were not present. The [Where it puts things](Where-it-puts-things) page
lists them.

If one of those paths is a symbolic link, shanty refuses to delete it and tells
you, rather than following the link and deleting something elsewhere.

It does not delete the shanty binary itself. That is wherever you installed it,
which is usually `~/.local/bin/shanty`.

## `shanty version`

Prints the version of the binary you are running. A build made from a clone of
the repository rather than from a release reports `dev`.

## `shanty help`

Lists the commands with a one-line description of each.
