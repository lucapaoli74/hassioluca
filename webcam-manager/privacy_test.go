package main

import (
	"bytes"
	"image"
	"image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPrivacyFacesAndZones(t *testing.T) {
	dir := os.Getenv("FACES_DIR")
	if dir == "" {
		t.Skip("FACES_DIR non impostata")
	}
	for _, name := range []string{"sample.jpg"} {
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		img, _, err := image.Decode(f)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		rgba := resizeToRGBA(img, 0)
		start := time.Now()
		faces := detectFaces(rgba, 2)
		t.Logf("%s %dx%d: %d volti in %v", name, rgba.Bounds().Dx(), rgba.Bounds().Dy(), len(faces), time.Since(start))
		if len(faces) == 0 {
			t.Errorf("%s: nessun volto trovato", name)
		}
		var src bytes.Buffer
		_ = jpeg.Encode(&src, img, nil)
		cam := Camera{Quality: 85, PrivacyFaces: true, PrivacySensitivity: 2, Overlays: []Overlay{
			{Type: "privacy", Enabled: true, Anchor: "bottom-right", Size: 20, Height: 15, Opacity: 1},
		}}
		out, boxes, _, err := NewRenderer("", nil).Render(src.Bytes(), cam, time.Now(), nil)
		if err != nil || boxes[0].Empty() {
			t.Fatalf("render: %v %v", boxes, err)
		}
		_ = os.WriteFile(filepath.Join(dir, "out-"+name+".jpg"), out, 0o644)
	}
}

func TestPrivacyNoFalseFaceOnMask(t *testing.T) {
	dir := os.Getenv("FACES_DIR")
	if dir == "" {
		t.Skip("FACES_DIR non impostata")
	}
	f, err := os.Open(filepath.Join(dir, "carnival.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, _, _ := image.Decode(f)
	if n := len(detectFaces(resizeToRGBA(img, 0), 2)); n != 0 {
		t.Fatalf("maschera scambiata per %d volti", n)
	}
}
