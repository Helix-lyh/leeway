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
// This is a trace of resources/icon.png's own ink, not a spiral drawn to look
// like it. The first attempt drew its own spiral with the same start angle and a
// chosen turn count, and it never matched: at this size the icon reads as a
// solid pinwheel with one slot cut through it, and a hand-drawn glyph reads as a
// thin outlined "6", because the ratio that decides which one you see — stroke
// against the gap between turns — is a third knob to guess at. Tracing removes
// the guessing. Change the icon, re-run resources/icon-glyph.py, and the two
// agree again by construction.
//
// It is a path with fill="currentColor" rather than the icon bitmap: a violet
// squircle tile beside the wordmark is a second app icon, and ui.Image cannot
// tint, so a bitmap would take one file per theme.
var (
	iconApp      = icon(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="currentColor" fill-rule="evenodd"><path d="M11.28 1.68L8.4 3.6L9.12 2.4L8.4 2.16L5.04 6L5.04 4.56L3.36 7.2L3.36 8.64L2.4 9.12L2.88 13.92L2.4 15.36L4.32 18.24L6 19.92L9.36 21.36L9.36 22.32L12.72 22.32L14.64 21.36L15.12 22.32L16.08 21.84L18.72 19.2L20.16 16.32L20.16 13.92L19.2 11.52L17.52 9.84L14.16 8.88L12.96 9.6L15.6 10.32L17.28 12L17.76 13.44L17.28 15.84L14.64 18.48L12.72 18.96L10.8 18.48L7.68 15.84L6.72 13.44L6.72 10.56L7.68 8.16L9.84 6L13.2 4.56L17.04 5.04L18.96 4.56L21.6 6.24L19.44 4.08L16.56 3.12L17.04 2.16L11.76 1.68Z"/></svg>`)
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
