# Images

`shanty_art.txt` is the artwork as characters: a hut on the shore, drawn in the
nine glyphs `# % * + - . : =` and `@`.

`shanty-source.png` is that text rendered — light glyphs on a dark ground.

`shanty-dark.png` and `shanty-light.png` are derived from it for the README.
The ground becomes transparent, or it would sit as a slab in whichever theme it
was not made for, and the glyphs are flooded with one colour: a bright cyan for
dark backgrounds, a deep slate for light ones. The README picks between them
with `<picture>` and `prefers-color-scheme`.

To regenerate after editing the source:

```sh
go run docs/images/derive.go
```

bothy does the same job with ImageMagick and a `-negate`, because its source is
dark ink on white and darkness has to become opacity. shanty's source is the
other way round, so brightness becomes opacity and there is nothing to negate.
This is Go rather than a shell script because Go is the language already here,
and because the whole transform is thirty lines of `image/png` — one clip to
remove the anti-aliasing haze that would otherwise ring every character, one
trim, one flood.

The two colours are provisional. shanty ships no palette yet — themes are v0.3
— and they will come from it when there is one.
