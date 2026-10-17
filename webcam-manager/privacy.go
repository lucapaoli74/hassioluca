package main

import (
	_ "embed"
	"image"
	"image/color"
	"log"
	"sync"

	pigo "github.com/esimov/pigo/core"
	xdraw "golang.org/x/image/draw"
)

// Privacy: zone fisse pixelate sempre (livelli "privacy" disegnati
// dall'utente) e volti riconosciuti automaticamente nell'immagine.
// Il riconoscimento usa pigo (MIT, vedi privacy/LICENSE-pigo), tutto in locale.

//go:embed privacy/facefinder
var faceCascade []byte

var (
	faceOnce       sync.Once
	faceClassifier *pigo.Pigo
)

func classifier() *pigo.Pigo {
	faceOnce.Do(func() {
		c, err := pigo.NewPigo().Unpack(faceCascade)
		if err != nil {
			log.Printf("riconoscimento volti non disponibile: %v", err)
			return
		}
		faceClassifier = c
	})
	return faceClassifier
}

// detectFaces restituisce i riquadri dei volti trovati. sensitivity 1-3:
// più alta = trova più volti (anche piccoli o incerti) ma con più falsi allarmi.
func detectFaces(img *image.RGBA, sensitivity int) []image.Rectangle {
	c := classifier()
	if c == nil {
		return nil
	}
	b := img.Bounds()
	cols, rows := b.Dx(), b.Dy()
	pixels := make([]uint8, cols*rows)
	for y := 0; y < rows; y++ {
		off := img.PixOffset(b.Min.X, b.Min.Y+y)
		for x := 0; x < cols; x++ {
			p := img.Pix[off+x*4 : off+x*4+3]
			pixels[y*cols+x] = uint8((299*int(p[0]) + 587*int(p[1]) + 114*int(p[2])) / 1000)
		}
	}
	minSize, threshold := 20, float32(5)
	switch sensitivity {
	case 1:
		minSize, threshold = 28, 8
	case 3:
		minSize, threshold = 14, 3
	}
	dets := c.RunCascade(pigo.CascadeParams{
		MinSize: minSize, MaxSize: min(cols, rows), ShiftFactor: 0.1, ScaleFactor: 1.1,
		ImageParams: pigo.ImageParams{Pixels: pixels, Rows: rows, Cols: cols, Dim: cols},
	}, 0)
	dets = c.ClusterDetections(dets, 0.2)
	var out []image.Rectangle
	for _, d := range dets {
		if d.Q < threshold {
			continue
		}
		// riquadro un po' più grande del volto, per coprire capelli e mento
		half := d.Scale * 7 / 10
		r := image.Rect(d.Col-half, d.Row-half, d.Col+half, d.Row+half*13/10).Intersect(b)
		if !r.Empty() {
			out = append(out, r)
		}
	}
	return out
}

// pixelate sostituisce l'area con blocchi di colore medio (irreversibile).
func pixelate(img *image.RGBA, r image.Rectangle) {
	r = r.Intersect(img.Bounds())
	if r.Empty() {
		return
	}
	block := max(6, min(r.Dx(), r.Dy())/6)
	for by := r.Min.Y; by < r.Max.Y; by += block {
		for bx := r.Min.X; bx < r.Max.X; bx += block {
			cell := image.Rect(bx, by, min(bx+block, r.Max.X), min(by+block, r.Max.Y))
			var sr, sg, sb, n int
			for y := cell.Min.Y; y < cell.Max.Y; y++ {
				for x := cell.Min.X; x < cell.Max.X; x++ {
					o := img.PixOffset(x, y)
					sr += int(img.Pix[o])
					sg += int(img.Pix[o+1])
					sb += int(img.Pix[o+2])
					n++
				}
			}
			if n == 0 {
				continue
			}
			avg := color.RGBA{uint8(sr / n), uint8(sg / n), uint8(sb / n), 255}
			xdraw.Draw(img, cell, image.NewUniform(avg), image.Point{}, xdraw.Src)
		}
	}
}

// applyPrivacy pixela le zone privacy e (se attivo) i volti. Restituisce i
// riquadri delle zone (per l'editor) e il numero di volti oscurati.
func applyPrivacy(img *image.RGBA, cam Camera, boxes []image.Rectangle) int {
	W, H := img.Bounds().Dx(), img.Bounds().Dy()
	for i, o := range cam.Overlays {
		if !o.Enabled || o.Type != "privacy" {
			continue
		}
		w := int(o.Size / 100 * float64(W))
		h := int(o.Height / 100 * float64(H))
		if w <= 0 || h <= 0 {
			continue
		}
		pos := place(o, W, H, w, h)
		r := image.Rect(pos.X, pos.Y, pos.X+w, pos.Y+h)
		pixelate(img, r)
		if i < len(boxes) {
			boxes[i] = r
		}
	}
	if !cam.PrivacyFaces {
		return 0
	}
	faces := detectFaces(img, cam.PrivacySensitivity)
	for _, f := range faces {
		pixelate(img, f)
	}
	return len(faces)
}
