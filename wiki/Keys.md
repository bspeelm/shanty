# Keys

shanty has three screens: a list of artists, the albums by one artist, and the
tracks on one album. You move down into them and back out again. The same keys
work on all three.

## Moving through a list

| Key | What it does |
|---|---|
| `↑` or `k` | Move the selection up one row |
| `↓` or `j` | Move the selection down one row |
| `g` or `home` | Jump to the first row |
| `G` or `end` | Jump to the last row |
| `pgup` or `pgdown` | Move up or down by one screenful |
| `enter`, `l` or `→` | Open the selected artist or album, or play the selected track |
| `esc`, `h`, `←` or `backspace` | Go back to the previous screen |

Pressing back on the artist list does nothing, because there is nothing above
it. This prevents you from quitting the player by pressing back once too often.

Each screen remembers which row you had selected. If you open an album and then
go back, the album you opened is still highlighted.

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

The volume ranges from 0 to 100 percent and stops at each end, so holding a key
down cannot push it past either limit.

When you start a track, shanty also tells mpv about the track after it. mpv
opens that file in advance, so an album plays through without a pause between
tracks.

## Quitting

| Key | What it does |
|---|---|
| `q` or `ctrl+c` | Quit shanty |

Quitting also stops mpv. shanty does not leave a player running in the
background.

## Changing the keys

The keys are currently fixed. Configurable key bindings are planned.
