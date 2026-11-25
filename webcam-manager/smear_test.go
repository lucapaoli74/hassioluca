package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLooksSmeared(t *testing.T) {
	dir := os.Getenv("SMEAR_DIR")
	if dir == "" {
		t.Skip("SMEAR_DIR non impostata")
	}
	want := map[string]bool{"user.jpg": true, "fog.jpg": false, "night.jpg": false, "bars.jpg": false, "scene.jpg": false, "busy.jpg": false}
	for f, exp := range want {
		b, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			t.Fatal(err)
		}
		if got := looksSmeared(b); got != exp {
			t.Errorf("%s: rovinata=%v, atteso %v", f, got, exp)
		}
	}
}
