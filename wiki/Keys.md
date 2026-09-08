# Keys

Everything shanty responds to. There is no keybinding configuration yet; it is
on the v0.2 list.

## Moving

| key | what it does |
|---|---|
| `↑` `k` | up one |
| `↓` `j` | down one |
| `g` `home` | first |
| `G` `end` | last |
| `pgup` `pgdown` | a screenful |
| `enter` `l` `→` | open — or play, on the track list |
| `esc` `h` `←` `backspace` | back one level |

Back at the top level does nothing. Leaving a music player by pressing left one
time too many is a surprise nobody wants mid-album.

The cursor is remembered per screen, so going back lands where you left.

## Playing

| key | what it does |
|---|---|
| `space` | pause or resume |
| `n` | next track |
| `p` | previous track |
| `]` | forward ten seconds |
| `[` | back ten seconds |
| `+` `=` | volume up five |
| `-` `_` | volume down five |

Volume clamps to 0–100, because the key can be held down.

Playing a track queues the one after it, which is what makes the gap between
tracks disappear — mpv opens the next file early rather than shanty decoding
anything.

## Leaving

| key | what it does |
|---|---|
| `q` `ctrl+c` | quit |

Quitting stops mpv. Nothing is left running.
