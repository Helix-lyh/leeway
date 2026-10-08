# Where the provider icons come from

These are the providers' own published icons, not drawings of ours.
`resources/provider-icons.py` fits each one to a square 256px PNG with the
corners rounded to the radius the window draws a mark with.

Seven marks have to read as one row, so they are all dark tiles with a light
glyph. Four providers publish exactly that and are used as published; three
publish the mark alone, so the script cuts the glyph out and the window draws
the same near-black tile behind it (`palette.markTile`).

| provider | file | what it is | source |
| --- | --- | --- | --- |
| kimi | `kimi.png` | the app icon: the K on black | <https://www.kimi.com/pwa-192.png> |
| grok | `grok.png` | the app icon: the mark on black | <https://grok.com/images/android-chrome-512x512.png> |
| cursor | `cursor.png` | the app icon: the cube on black | <https://cursor.com/apple-touch-icon.png> |
| commandcode | `commandcode.png` | the knot on black | <https://commandcode.ai/apple-touch-icon.png> |
| deepseek | `deepseek.png` | the whale, in its own blue, on nothing | <https://www.deepseek.com/favicon.ico> (its 225px frame) |
| codex | `codex.png` | the OpenAI knot in white, on nothing | the maintainer's reference image, `codex-knot.jpg` |
| opencode-go | `opencode-go.png` | the terminal mark, in its own two tones, on nothing | <https://opencode.ai/apple-touch-icon.png> |

## What the window does with them

- `mark()` draws the icon in a rounded square, with the state of the last probe
  as a dot at its corner, and a soft shadow so a white icon still reads as a
  tile on a white card.
- The marks that arrive as a glyph alone are listed in `glyphMarks`; the window
  draws its tile behind them and the script has already sized the glyph to 0.62
  of the side, which is where the providers that do publish a tile draw theirs.
- A provider we have no icon for falls back to its initial on a tint of its own
  hue. `TestEveryProviderHasItsOwnIcon` fails if that ever happens quietly.

## Changing one

Icons are trademarks of their providers and are used here to identify them. To
refresh one, put the provider's published asset in a directory and run:

```sh
python3 resources/provider-icons.py --raw /tmp/icons
```

The script prints each file it writes. Commit the resulting PNG in
`internal/gui/icons/`; it is embedded in the binary (`internal/gui/icons.go`),
so the window needs no network and no files beside it.
