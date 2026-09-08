## q

Quits shanty, and stops the music with it.

Quitting is a command rather than a key so that it is not one keystroke away
from every screen. Pressing `q` on its own says this rather than quitting.

`ctrl+c` also quits, because it is what people try when a program will not let
go.

## search

Looks for artists, albums and tracks anywhere on your server.

    :search slipway

`/` narrows the list in front of you and cannot reach what is not on it. Tracks
are loaded one album at a time, so a track on an album you have not opened is
not there to be filtered. This asks the server instead.

The results are the three kinds together under headings. `enter` opens an
artist or an album, or plays a track. `esc` returns to the artist list.

## playlist

Lists your playlists, or does one thing to one of them.

    :playlist
    :playlist create Evening
    :playlist edit Evening
    :playlist delete Evening
    :playlist shuffle Evening

`create` makes an empty playlist. `edit` shows your library with `(A)` beside
the tracks already in the playlist; `a` adds the selected track and `r` takes
it out, each reaching the server as you press it. `esc` finishes.

`delete` asks before it acts and names the playlist. Anything but `y` answers
no.

If two of your playlists share a name, shanty says so rather than guessing.
Rename one on your server.

## shuffle

Plays everything on your server in a random order.

It shuffles once, into a queue you can look at with `gq`. It is not a mode that
keeps reshuffling. `:playlist shuffle <name>` does the same to one playlist.

## scan

Asks your server to look at its music folder for new files.

Reloading finds nothing the server has not looked at, so music you have just
copied onto it needs this first. The last row counts the tracks as it goes, and
the library is reloaded by itself when the scan finishes.

A scan takes minutes on a large library. The music keeps playing and you can
carry on browsing.

Not every server offers this, and starting a scan often needs an administrator
account. shanty says which of the two it ran into.

## reload

Asks the server again for whatever screen you are on.

A list is fetched once and kept, so an album added to the server after shanty
started is not there. This asks again for the artist list, one artist's albums,
one album's tracks, or the starred list.

The cursor stays on what you had selected if it is still there, and the last
row says what came back.

## messages

Shows everything shanty has said this session, newest first, with the time.

The last row holds one message until the next replaces it, so something that
went wrong while you were reading a track list is otherwise gone. This is kept
in memory only: closing shanty forgets it.

## resume

Carries on from the queue your server is holding.

Your server keeps one queue for your whole account, so what you were playing on
another machine can be picked up here. shanty saves it when a track changes and
when you quit.

When there is one to take up, the last row says so and names who left it. It is
offered rather than applied, because quitting is usually deliberate.

## headless

Closes the interface and keeps playing.

The shell prompt comes back and the album carries on, with your server still
told what you listened to. `shanty status`, `shanty pause`, `shanty next` and
the rest command it from any shell, and running `shanty` again returns the
interface to it.

It ends by itself when the queue runs out. `:headless` with nothing playing is
refused.

## volume

Sets the volume to a number from 0 to 100.

    :volume 40

`+` and `-` change it by five at a time. A number outside the range is reported
rather than rounded to the nearest end: holding a key down should stop at the
end, but typing 400 is a mistake worth being told about.

## wiki

Shows this.

`enter` on a command explains it, and `esc` goes back to the list.
