package main

import (
	"image"
	"image/color"
	"path/filepath"
	"testing"
)

func TestFitSize(t *testing.T) {
	cases := []struct {
		sw, sh, maxW, maxH int
		noUp               bool
		w, h               int
	}{
		{4000, 3000, 1000, 1000, false, 1000, 750},
		{800, 1200, 1000, 1000, false, 667, 1000},
		{300, 200, 1000, 0, false, 1000, 667},
		{300, 200, 1000, 0, true, 300, 200},
		{4000, 3000, 0, 300, false, 400, 300},
	}
	for _, c := range cases {
		w, h := fitSize(c.sw, c.sh, c.maxW, c.maxH, c.noUp)
		if w != c.w || h != c.h {
			t.Errorf("fitSize(%d,%d,%d,%d,%v) = %dx%d, vrea %dx%d", c.sw, c.sh, c.maxW, c.maxH, c.noUp, w, h, c.w, c.h)
		}
	}
}

func TestPadAndFillExactSize(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 400, 100))
	cfg := config{maxW: 200, maxH: 200, bg: color.NRGBA{255, 255, 255, 255}}
	if b := pad(src, cfg).Bounds(); b.Dx() != 200 || b.Dy() != 200 {
		t.Errorf("pad = %v", b)
	}
	if b := fill(src, 200, 200).Bounds(); b.Dx() != 200 || b.Dy() != 200 {
		t.Errorf("fill = %v", b)
	}
}

func TestParseColor(t *testing.T) {
	if c, err := parseColor("#ff8000"); err != nil || c != (color.NRGBA{255, 128, 0, 255}) {
		t.Errorf("parseColor(#ff8000) = %v, %v", c, err)
	}
	if c, err := parseColor("transparent"); err != nil || c.A != 0 {
		t.Errorf("parseColor(transparent) = %v, %v", c, err)
	}
	if _, err := parseColor("fff"); err == nil {
		t.Error("parseColor(fff) trebuia să dea eroare")
	}
}

func TestOutExt(t *testing.T) {
	cases := map[[2]string]string{
		{"a.JPG", ""}:    ".jpg",
		{"a.jpeg", ""}:   ".jpeg",
		{"a.gif", ""}:    ".png",
		{"a.webp", ""}:   ".png",
		{"a.png", "jpg"}: ".jpg",
	}
	for in, want := range cases {
		if got := outExt(in[0], in[1]); got != want {
			t.Errorf("outExt(%q,%q) = %q, vrea %q", in[0], in[1], got, want)
		}
	}
}

func TestPlanOutputsNoCollision(t *testing.T) {
	cfg := config{outDir: "/out"}
	jobs := planOutputs([]string{"/in/c.gif", "/in/c.png", "/in/a.jpg"}, cfg)
	seen := map[string]bool{}
	for _, j := range jobs {
		if seen[j.dst] {
			t.Fatalf("destinație duplicată: %s", j.dst)
		}
		seen[j.dst] = true
	}
	if jobs[2].dst != filepath.Join("/out", "a.jpg") {
		t.Errorf("fișierele fără coliziune nu trebuie redenumite: %s", jobs[2].dst)
	}
}

func TestApplyOrientation6(t *testing.T) {
	// 2x1: pixel roșu stânga, albastru dreapta; orientarea 6 = rotire 90° în sensul acelor
	src := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	src.Set(0, 0, color.NRGBA{255, 0, 0, 255})
	src.Set(1, 0, color.NRGBA{0, 0, 255, 255})
	out := applyOrientation(src, 6).(*image.NRGBA)
	if b := out.Bounds(); b.Dx() != 1 || b.Dy() != 2 {
		t.Fatalf("dimensiuni = %v", b)
	}
	if out.NRGBAAt(0, 0).R != 255 || out.NRGBAAt(0, 1).B != 255 {
		t.Error("rotire greșită")
	}
}
