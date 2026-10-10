package gui

import "testing"

// A crest that only partly covers its row used to be drawn at that fraction
// of the ink. On the dark well that dot is missing or darker than the
// lattice, and a slope turns the fringe into a black shadow.
func TestTheDotCrestIsADotNotAShadow(t *testing.T) {
	if dotOn(10, 10.2, true) {
		t.Fatal("a row the reading only nicks is a ghost, darker than the lattice")
	}
	if !dotOn(9, 10.2, true) {
		t.Fatal("the row the reading has passed should be lit")
	}
	if !dotOn(10, 10.5, true) {
		t.Fatal("a reading on the row's centre should light it")
	}
	if !dotOn(0, 0, true) {
		t.Fatal("a reading at the floor still marks the baseline")
	}
	if dotOn(1, 0, true) {
		t.Fatal("a reading at the floor does not fill the field")
	}
	if dotOn(0, 10, false) {
		t.Fatal("a column with no number stays lattice")
	}
}
