package main

import "testing"

func TestAppMarkIsASquareSVGMygoFullyRenders(t *testing.T) {
	if w, h := appMark.Size(); w != h || w == 0 {
		t.Errorf("app mark is %vx%v, want a square", w, h)
	}
	if unsupported := appMark.Unsupported(); len(unsupported) != 0 {
		t.Errorf("mygo cannot draw %v of the app mark", unsupported)
	}
}
