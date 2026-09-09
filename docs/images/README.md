# Images

`shanty_art.txt` is the artwork as characters: a hut on the shore, drawn in the
nine glyphs `# % * + - . : =` and `@`.

`shanty-source.png` is that text rendered — light glyphs on a dark ground.

`shanty_screenshot.png` is the album screen: a cover drawn with the kitty
graphics protocol, the album's facts beside it, and the track list below. It is
a photograph of a terminal rather than anything generated, so it is replaced by
taking another one.

`shanty-dark.png` and `shanty-light.png` are derived from it for the README.
The ground becomes transparent, or it would sit as a slab in whichever theme it
was not made for, and the glyphs are flooded with one colour: a bright cyan for
dark backgrounds, a deep slate for light ones. The README picks between them
with `<picture>` and `prefers-color-scheme`.

To regenerate after editing the source:

```sh
go run docs/images/derive.go
```

The script reads `shanty-source.png`, turns brightness into opacity, clips the
anti-aliasing that would otherwise leave a halo around each character, trims
the empty border, and fills the result with one colour.

The two colours are provisional and will come from shanty's palette once it
has one.
