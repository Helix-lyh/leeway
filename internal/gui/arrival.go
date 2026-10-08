package gui

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// The first frame of a window places its blocks in order: the head, then the
// cards under it, each one a step behind the last. It is the window's own
// arrival, so it runs once, when the window opens.
const (
	arrivalStep = 55 * time.Millisecond
	arrivalDur  = 400 * time.Millisecond
	arrivalRise = 10 // DIPs the block comes up from
)

// arrival anchors that entrance: when the window opened, and how many blocks
// the frame being built has placed.
type arrival struct {
	at    time.Time
	index int
}

// hold eases one block of a page in behind the one before it, as the window's
// clock reads. Blocks count in the order the frame builds them, so a page
// reads top to bottom.
//
// The block goes inside an element of the entrance's own: a card carries a
// transition that brings it in as a page changes, and that would write over
// the fade this sets.
func (ar *arrival) hold(c *ui.Context, now time.Time, block func()) *ui.Element {
	holder := ui.Column(c).FillWidth()
	// A window that never opened, or a desktop asking for less motion: the
	// block is at its place from the first frame.
	if !ar.at.IsZero() && !c.Preferences().ReduceMotion {
		if p := arrivalProgress(now.Sub(ar.at), ar.index); p < 1 {
			c.AnimationFrame()
			// It starts low and comes up into its place.
			holder.Margin((1-p)*arrivalRise, 0, 0, 0).Opacity(p)
		}
	}
	ar.index++
	return holder.Children(block)
}

// arrivalProgress is how far the block at index is into its entrance, from 0
// when it has not started to 1 when it has arrived.
func arrivalProgress(elapsed time.Duration, index int) float32 {
	t := elapsed - time.Duration(index)*arrivalStep
	switch {
	case t <= 0:
		return 0
	case t >= arrivalDur:
		return 1
	default:
		return easeGlide(float32(t) / float32(arrivalDur))
	}
}
