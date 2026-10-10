package gui

import (
	"embed"

	"github.com/egoist/mygo/ui"
)

// Icons are Lucide paths in a 24x24 box, drawn in the color of the text
// around them. ui.Icon sizes them with the font.
func icon(svg string) *ui.SVG { return ui.MustParseSVG([]byte(svg)) }

// iconApp is the app's own mark, so it is the app icon: the brush stroke wound
// into a spiral, its tail coming apart into a flick. It used to be three bars of
// decreasing length — a quota bar, the one thing the mark must not be, since a
// bar is a UI component and the window already draws the real one a row below.
//
// The spiral is resources/icon.py's, re-fitted for a glyph that has to survive
// being rasterised at 16px beside the wordmark: 0.95 turns rather than the
// icon's 1.10, so the counter stays open at that size, and one flick rather than
// the icon's whole spread of hairs. The weight was picked by rendering
// candidates through MyGo's own rasteriser and looking at them, not by preview —
// resources/icon-glyph.py regenerates this path.
var (
	iconApp      = icon(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><path d="M13.53 11.41C13.8 11.4 14.62 11.2 15.16 11.34C15.71 11.47 16.35 11.77 16.81 12.21C17.27 12.65 17.72 13.31 17.92 14C18.12 14.68 18.19 15.56 18 16.33C17.82 17.1 17.4 17.98 16.81 18.61C16.22 19.25 15.35 19.86 14.45 20.14C13.55 20.43 12.41 20.55 11.41 20.34C10.4 20.14 9.27 19.64 8.44 18.93C7.6 18.22 6.8 17.16 6.39 16.06C5.97 14.95 5.78 13.55 5.96 12.31C6.14 11.07 6.68 9.67 7.49 8.61C8.3 7.55 10.25 6.4 10.81 5.95M10.81 5.95L13.12 3.56M10.81 5.95L11.15 4.64"/></svg>`)
	iconOverview = icon(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M22 12h-4l-3 9L9 3l-3 9H2"/></svg>`)
	iconHistory  = icon(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>`)
	iconAbout    = icon(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="M12 16v-4"/><path d="M12 8h.01"/></svg>`)
	iconSettings = icon(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="4" y1="21" x2="4" y2="14"/><line x1="4" y1="10" x2="4" y2="3"/><line x1="12" y1="21" x2="12" y2="12"/><line x1="12" y1="8" x2="12" y2="3"/><line x1="20" y1="21" x2="20" y2="16"/><line x1="20" y1="12" x2="20" y2="3"/><line x1="1" y1="14" x2="7" y2="14"/><line x1="9" y1="8" x2="15" y2="8"/><line x1="17" y1="16" x2="23" y2="16"/></svg>`)
	iconRefresh  = icon(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="23 4 23 10 17 10"/><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/></svg>`)
	iconChevron  = icon(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="9 18 15 12 9 6"/></svg>`)
	iconMore     = icon(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="currentColor"><circle cx="5" cy="12" r="2"/><circle cx="12" cy="12" r="2"/><circle cx="19" cy="12" r="2"/></svg>`)
	iconAlert    = icon(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>`)
	iconKeyhole  = icon(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="9"/><circle cx="12" cy="10" r="2"/><path d="M12 12v4"/></svg>`)
)

// Provider marks are the providers' own icons, embedded as published: a window
// that watches seven subscriptions is read by their logos, and a letter we
// drew is not one of them. resources/provider-icons.py turns whatever each
// provider publishes — a 16-bit ICO, a full-bleed app icon, a glyph on
// nothing — into these squares; icons/SOURCES.md lists where each came from.
//
//go:embed icons/*.png
var iconFiles embed.FS

// marks holds the decoded icons. Decoding one costs a few milliseconds and a
// mark never changes while the window is up, so each is decoded at most once.
var marks = map[string]*ui.Bitmap{}

// glyphMarks names the providers whose icon is the mark alone, with no tile of
// its own: the window draws the tile, so that a blue whale and a black knot sit
// in the same dark squares as the icons the providers do publish as tiles.
var glyphMarks = map[string]bool{"deepseek": true, "codex": true, "opencode-go": true}

// providerMark is a provider's icon, or nil when we have none for it.
func providerMark(id string) *ui.Bitmap {
	if b, ok := marks[id]; ok {
		return b
	}
	var b *ui.Bitmap
	if data, err := iconFiles.ReadFile("icons/" + id + ".png"); err == nil {
		b, _ = ui.DecodeBitmap(data)
	}
	marks[id] = b
	return b
}
