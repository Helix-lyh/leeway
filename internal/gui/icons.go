package gui

import (
	"embed"

	"github.com/egoist/mygo/ui"
)

// Icons are Lucide paths in a 24x24 box, drawn in the color of the text
// around them. ui.Icon sizes them with the font.
func icon(svg string) *ui.SVG { return ui.MustParseSVG([]byte(svg)) }

var (
	iconApp      = icon(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round"><path d="M5 8h14"/><path d="M5 12h9"/><path d="M5 16h5"/></svg>`)
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
