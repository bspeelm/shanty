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
    :playlist rename Evening Watch
    :playlist reorder
    :playlist delete Evening
    :playlist shuffle Evening

`gp` opens the list of playlists and `enter` opens one. From there `e` starts
adding to that playlist without naming it, and `r` takes the track under the
cursor out of it.

`create` makes an empty playlist. `edit` shows your library with `(A)` beside
the tracks already in the playlist; `a` adds the selected track and `r` takes
it out, each reaching the server as you press it. `e` finishes.

`reorder` rearranges the playlist you are looking at: `K` and `J` move the
selected track, `enter` saves the arrangement and `esc` forgets it. Nothing
reaches the server until you press `enter`, and if the playlist changed there
in the meantime the change is refused rather than overwriting it.

`delete` with no name deletes the playlist you are looking at.

`rename` is the one verb whose argument is not a playlist. It renames the
playlist you are looking at, and what you type is the new name, so open one
first.

`delete` asks before it acts and names the playlist. Anything but `y` answers
no.

If two of your playlists share a name, shanty says so rather than guessing
which you mean. Open the one you want and use `rename`.

## visualizer

Fills the screen with moving pictures that follow the music.

    :visualizer

`gv` opens it too, `esc` puts it away, and the row above the keys still says
what is playing and how far through it is, so you can leave it up.

Each scene is made of a few layers, such as spirals, tunnels, rings,
star fields, flocks, fractals and cellular automata, drawn through mirrors,
twists and kaleidoscopes, in a palette of its own. The bass, the treble, the
hiss, the stereo balance, the beat and how busy the music is each drive part
of the scene, and which part changes from scene to scene. Layers also react
to one another: a flock is drawn towards what is bright, the Game of Life
grows where the picture is lit. What it draws is lined up with what you hear.

It listens for the character of the music and draws to suit it. Quiet,
sustained music gets slow, deep-coloured scenes with long trails; a steady
dance beat gets bright, hard-edged scenes that step on the beat; loud, bright,
busy music gets fire, sparks and flocks in hot colours; music that swells and
falls gets flowing lines and figures in warm colours. Most music is a mixture,
and the scenes are too. It takes about fifteen seconds of a track to judge.

Scenes grow into one another: some layers stay, carrying on where they were,
while others fade out and new ones fade in, and the colours and the way of
drawing shift across. When the music changes character, or a new track sounds
quite different, a new scene fades in instead. Scenes are made fresh each
time, so they do not repeat in the same order. Now and then a scene is a
single classic algorithm on its own, and its name shows in the corner for a
few seconds.

It follows the shape of the music as well as its sound. A climb in a song
gathers the picture in, darker and closer; when it breaks or drops, light
blooms out; a quiet passage is allowed to be dark and still. The picture
breathes slowly with the phrases. While it is open, shanty measures the song
playing and the next one ahead of time, so it can build towards a drop before
you hear it and change scene as a new section begins. From that measuring it
takes the beat, pulses on each kick, cuts to a new scene at a drop, goes soft
where the kicks stop, and draws loud music without a steady kick faster,
brighter and shaking.

It limits how fast large parts of the picture change brightness, to at most
about two flashes a second. The limit is measured for a dark terminal
background in full colour.

Some scenes are landscapes: a lake under the moon, a sea at dawn, a storm, an
aurora. Their light and weather follow the music: cloud gathers as it builds,
light breaks through as it resolves, snow and stars come in the quiet.

Pausing holds the picture where it is.

## shuffle

Plays everything on your server in a random order.

It shuffles once, into a queue you can look at with `gq`. It is not a mode that
keeps reshuffling. `:playlist shuffle <name>` does the same to one playlist.

## resume

Carries on from the queue your server is holding.

Your server keeps one queue for your whole account, so what you were playing on
another machine can be picked up here. shanty saves it when a track changes and
when you quit.

When there is one to take up, the last row says so and names who left it. It is
offered rather than applied, because quitting is usually deliberate.

## art

Shows the cover of the album you are looking at, as large as the screen allows.

    :art

`esc` puts it away and returns you to the track list.

Above the track list a cover takes twelve rows, and it is not drawn at all when
that would leave eight tracks or fewer on screen. This is how to see the whole
thing on a short terminal, and how to look at it properly on any terminal.

It follows the window: make the terminal bigger and the cover grows with it.
Below twenty columns by fifteen rows there is no room for one, and it says so
rather than drawing something smaller than the cover it replaced.

It needs an album open, and a terminal that draws pictures. One that does not
shows no covers anywhere and says so here. `shanty doctor` names what it found
under `cover-art`.

## auto-vol

Levels quiet records against loud ones, so that shuffling does not jump in
volume between a vinyl rip and a modern master.

    :auto-vol on
    :auto-vol off
    :auto-vol          changes it to whichever it is not

It is remembered in `config.toml` and comes back the way you left it.

It uses the loudness a tagger measured and wrote into each track. That is a
number, not a filter: the track is turned up or down as a whole and its
dynamics are untouched. A track nobody has measured is played exactly as it
is, so a part-tagged library is safe.

The measuring is not done by shanty and not by the server. It is written into
the files on whatever holds your music, with a tool such as rsgain.

**Turning it on and hearing no difference means one of two things**, and shanty
cannot tell you which. Either nothing needed correcting, or nothing in your
library has been measured and there is nothing to read. Both are silence.

The answer is in the files rather than here. On the machine holding your music,
a track that has been measured carries a `REPLAYGAIN_TRACK_GAIN` tag; one that
has not carries nothing. If none of them do, this command has nothing to work
with however it is set.

## wiki

Shows this.

`enter` on a command explains it, and `esc` goes back to the list.

## messages

Shows everything shanty has said this session, newest first, with the time.

The last row holds one message until the next replaces it, so something that
went wrong while you were reading a track list is otherwise gone. This is kept
in memory only: closing shanty forgets it.

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

## volume

Sets the volume to a number from 0 to 100.

    :volume 40

`+` and `-` change it by five at a time. A number outside the range is reported
rather than rounded to the nearest end: holding a key down should stop at the
end, but typing 400 is a mistake worth being told about.

## headless

Closes the interface and keeps playing.

The shell prompt comes back and the album carries on, with your server still
told what you listened to. `shanty status`, `shanty pause`, `shanty next` and
the rest command it from any shell, and running `shanty` again returns the
interface to it.

It ends by itself when the queue runs out. `:headless` with nothing playing is
refused.

## q

Quits shanty, and stops the music with it.

Quitting is a command rather than a key so that it is not one keystroke away
from every screen. Pressing `q` on its own says this rather than quitting.

`ctrl+c` also quits, because it is what people try when a program will not let
go.
