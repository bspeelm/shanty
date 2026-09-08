# Keys

shanty has three screens: a list of artists, the albums by one artist, and the
tracks on one album. You move down into them and back out again. The same keys
work on all three.

## Moving through a list

| Key | What it does |
|---|---|
| `↑` or `k` | Move the selection up one row |
| `↓` or `j` | Move the selection down one row |
| `gg` or `home` | Jump to the first row |
| `G` or `end` | Jump to the last row |
| `pgup` or `pgdown` | Move up or down by one screenful |
| a number before a movement | Repeat it, so `3j` moves down three |
| `enter`, `l` or `→` | Open the selected artist or album, or play the selected track |
| `esc`, `h`, `←` or `backspace` | Go back to the previous screen |
| `/` | Filter the list you are looking at |
| `a` | Add the selected track to the end of the queue |
| `A` | Play the selected track after the one playing |
| `*` | Star the selected artist, album or track, or unstar it |

Pressing back on the artist list does nothing, because there is nothing above
it. This prevents you from quitting the player by pressing back once too often.

Each screen remembers which row you had selected. If you open an album and then
go back, the album you opened is still highlighted.

## Seeing music you have just added

shanty asks the server for a list once and then keeps it, so an album copied
onto the server after you started shanty is not there.

`:reload` asks again for whatever screen you are on: the artist list, one
artist's albums, one album's tracks, or the starred list. It says what came
back, because a reload that returns the same thing looks exactly like one that
did nothing.

The cursor stays on what you had selected, if it is still there. It reloads
only the screen you are on rather than the whole library, which on a large one
would take a while to see one new album; the screens above are asked for again
when you next open them.

If you have just copied files onto the server, it has not looked at them yet
and reloading will not find them. `:scan` asks it to look. The last row reports
how many tracks it has got through, and the library is reloaded by itself when
the scan finishes, so there is no second command to remember.

A scan takes minutes on a large library. The music keeps playing throughout and
you can carry on browsing; shanty asks the server how it is going every couple
of seconds rather than waiting for it.

Not every server offers this, and starting a scan often needs an administrator
account. shanty says which of the two it ran into rather than reporting a
failure of its own.

## What shanty said earlier

The last row shows one message until the next replaces it, so something that
went wrong while you were reading a track list is gone by the time you look.

`:messages` lists everything shanty has said this session, newest first, with
the time. It is kept in memory only: closing shanty forgets it.

## Carrying on from another machine

Your server keeps a queue and the position in it, so what you were playing on
one machine can be picked up on another. shanty saves it when a track changes
and when you quit.

When you start shanty and your server is holding a queue, the last line says so
and names who left it — you, or another client by name. Type `:resume` to take
it up, on the track and at the second it was left.

It is offered rather than applied, because quitting is usually deliberate and
having the music start again by itself would be a surprise. Nothing is offered
if something is already playing.

**Your server keeps one queue for your whole account, not one per machine.**
Every client that supports this writes to the same place, so the queue you are
offered is whichever one saved last. While shanty is playing it saves on every
track change, which means it overwrites what your phone left; your phone will
do the same to shanty. This is how the feature works everywhere rather than
anything particular to shanty, and it is worth knowing before you rely on it.

A client that does not support this leaves nothing for shanty to offer, however
long you played on it.

## Searching the server

`/` narrows the list in front of you. It cannot reach what is not on it, and
tracks are loaded one album at a time, so a track on an album you have not
opened is not there to be filtered.

`:search slipway` asks the server instead. The results are artists, albums and
tracks together, grouped under headings, with the count in the title bar.
`enter` opens an artist or an album, or plays a track. `esc` returns to the
artist list.

The headings are not rows: moving passes over them, and filtering the results
with `/` drops them.

Searching needs the whole query before it can start, which is why it is a
command rather than a key like `/`.

## Filtering a list

Press `/` and type. The list narrows to rows containing what you typed,
matching anywhere in the name and ignoring case. The arrow keys still move
while you are typing.

`enter` keeps the filter and returns you to normal movement. `esc` abandons it
and restores the whole list. Opening an artist or album clears it, because a
filter belongs to the list it narrowed.

If nothing matches, the screen says so rather than going blank.

## Going to a screen

`g` begins these; press it and then the second key.

| Key | What it does |
|---|---|
| `ga` | Artists |
| `gq` | The queue, opened on the track playing |
| `gp` | Playlists |
| `gs` | Starred |

The playlists screen is not built yet. Pressing its key says so.

## Starred

`*` stars whatever is selected, and unstars it if it is starred already. It
works on artists, albums and tracks, and on any screen that lists them.

A star is kept on your server, not by shanty, so what you star here is starred
in every other client you use.

`★` before a name means it is starred. `gs` lists everything that is, grouped
the same way search results are.

## The queue

`gq` shows what is playing and what follows it, with the playing track marked.
`enter` on a row plays it, and `esc` returns to the artist list.

`a` and `A` add the track you have selected in an album. `a` puts it at the
end; `A` plays it after the track playing now, without interrupting it. Adding
to an empty queue starts it playing, because there is nothing else sensible to
do with a track you asked to hear next when nothing is on.

The queue is what a session plays after `:headless`, and what the interface
picks back up when you return to it.

## Controlling playback

These work on any screen, whatever you are browsing.

| Key | What it does |
|---|---|
| `space` | Pause, or resume if already paused |
| `n` | Skip to the next track |
| `p` | Go back to the previous track |
| `]` | Seek forward ten seconds |
| `[` | Seek backward ten seconds |
| `+` or `=` | Increase the volume by five percent |
| `-` or `_` | Decrease the volume by five percent |
| a number then `%` | Seek to that percentage, so `50%` goes to the middle |

The volume ranges from 0 to 100 percent and stops at each end, so holding a key
down cannot push it past either limit.

When you start a track, shanty also tells mpv about the track after it. mpv
opens that file in advance, so an album plays through without a pause between
tracks.

## Commands

Press `:` to type a command. The commands matching what you have typed are
listed above the line, so pressing `:` on its own shows all of them. `tab`
completes as far as the matches agree, `enter` runs, and `esc` abandons the
line.

| Command | What it does |
|---|---|
| `:q` | Quit |
| `:volume 40` | Set the volume to a number, where `+` and `-` change it by steps |
| `:headless` | Close the interface and keep playing |
| `:search slipway` | Find artists, albums and tracks on the server |
| `:resume` | Carry on from the queue saved on your server |
| `:messages` | Show what shanty has said this session |
| `:reload` | Ask the server for what is on screen again |
| `:scan` | Ask the server to look for new music |

Commands exist for things a key cannot do: those that need something typed
after them, and those too rare to be worth a key.

## Leaving the interface running

Type `:headless` and the interface closes, the shell prompt comes back, and the
music keeps playing. The album carries on to its end, and your server is told
what you listened to exactly as it would have been with the interface open.

These command what is left, from any shell:

| Command | What it does |
|---|---|
| `shanty status` | Say what is playing |
| `shanty play` | Resume |
| `shanty pause` | Pause |
| `shanty next` | Skip to the next track |
| `shanty prev` | Go back to the previous track |
| `shanty vol 40` | Set the volume, from 0 to 100 |
| `shanty seek 1:23` | Move to a position in the track |
| `shanty stop` | End it, and the music with it |

Run `shanty` again and the interface comes back on the track that is playing,
without a gap in the sound. You start on the artist list rather than the screen
you left, because a filter and a cursor belong to a screen rather than to the
music.

It ends by itself when the album finishes. Nothing is left running afterwards,
and nothing starts it except typing `:headless`.

`:headless` with nothing playing is refused, because such a session would end
the moment it started.

## Quitting

| Key | What it does |
|---|---|
| `:q` | Quit shanty |
| `ctrl+c` | Quit shanty |

Quitting is a command rather than a key. It ends the session and cannot be
undone by pressing something else, so it is not one keystroke away from every
screen. Pressing `q` tells you this rather than doing nothing.

Quitting stops mpv with it. Nothing is left running unless you asked for it
with `:headless`, which is the one way to keep the music going without the
interface.

## Changing the keys

The keys are currently fixed. Configurable key bindings are planned.
