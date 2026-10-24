// webrt-img-resize — uniformizează imaginile dintr-un folder la dimensiuni maxime date.
//
// Moduri:
//
//	fit  – încadrează în maxw×maxh păstrând proporțiile (dimensiunile finale pot varia)
//	pad  – încadrează + completează cu fundal → TOATE ies exact maxw×maxh
//	fill – umple complet maxw×maxh și decupează centrat → TOATE ies exact maxw×maxh
//
// Originalele nu sunt atinse niciodată: rezultatul merge în alt folder.
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

type config struct {
	maxW, maxH int
	mode       string
	quality    int
	format     string
	noUp       bool
	bg         color.NRGBA
	outDir     string
}

var supported = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true}

func main() {
	var cfg config
	var inDir, bgHex string
	var workers int

	flag.IntVar(&cfg.maxW, "maxw", 0, "lățimea maximă (px)")
	flag.IntVar(&cfg.maxH, "maxh", 0, "înălțimea maximă (px)")
	flag.StringVar(&cfg.mode, "mode", "fit", "fit | pad | fill")
	flag.StringVar(&inDir, "in", ".", "folderul sursă")
	flag.StringVar(&cfg.outDir, "out", "", "folderul destinație (implicit: <in>/resized)")
	flag.IntVar(&cfg.quality, "q", 85, "calitate JPEG 1-100")
	flag.StringVar(&cfg.format, "format", "", "forțează formatul de ieșire: jpg | png (implicit: păstrează)")
	flag.BoolVar(&cfg.noUp, "noup", false, "nu mări imaginile mai mici (fit/pad)")
	flag.StringVar(&bgHex, "bg", "ffffff", "fundal pentru pad / JPEG cu transparență: hex sau 'transparent'")
	flag.IntVar(&workers, "workers", runtime.NumCPU(), "câte imagini procesează în paralel")
	flag.Parse()

	switch cfg.mode {
	case "fit":
		if cfg.maxW <= 0 && cfg.maxH <= 0 {
			fail("dă cel puțin -maxw sau -maxh")
		}
	case "pad", "fill":
		if cfg.maxW <= 0 || cfg.maxH <= 0 {
			fail("modul %s cere și -maxw și -maxh", cfg.mode)
		}
	default:
		fail("mod necunoscut %q (fit | pad | fill)", cfg.mode)
	}
	if cfg.format != "" && cfg.format != "jpg" && cfg.format != "png" {
		fail("format necunoscut %q (jpg | png)", cfg.format)
	}
	if cfg.quality < 1 || cfg.quality > 100 {
		fail("-q trebuie să fie între 1 și 100")
	}
	if workers < 1 {
		workers = 1
	}
	bg, err := parseColor(bgHex)
	if err != nil {
		fail("%v", err)
	}
	cfg.bg = bg

	absIn, err := filepath.Abs(inDir)
	if err != nil {
		fail("%v", err)
	}
	if cfg.outDir == "" {
		cfg.outDir = filepath.Join(absIn, "resized")
	}
	absOut, err := filepath.Abs(cfg.outDir)
	if err != nil {
		fail("%v", err)
	}
	if absOut == absIn {
		fail("folderul destinație nu poate fi același cu sursa (ar suprascrie originalele)")
	}
	cfg.outDir = absOut

	entries, err := os.ReadDir(absIn)
	if err != nil {
		fail("%v", err)
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if supported[strings.ToLower(filepath.Ext(e.Name()))] {
			files = append(files, filepath.Join(absIn, e.Name()))
		}
	}
	if len(files) == 0 {
		fmt.Println("Nicio imagine (jpg/png/gif/webp) în", absIn)
		return
	}
	if err := os.MkdirAll(absOut, 0o755); err != nil {
		fail("%v", err)
	}

	fmt.Printf("%d imagini → %s  [mode=%s, %dx%d]\n", len(files), absOut, cfg.mode, cfg.maxW, cfg.maxH)

	var ok, bad int64
	jobs := make(chan job)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				if err := process(j, cfg); err != nil {
					atomic.AddInt64(&bad, 1)
					fmt.Fprintf(os.Stderr, "✗ %s: %v\n", filepath.Base(j.src), err)
				} else {
					atomic.AddInt64(&ok, 1)
				}
			}
		}()
	}
	for _, j := range planOutputs(files, cfg) {
		jobs <- j
	}
	close(jobs)
	wg.Wait()

	fmt.Printf("\nGata: %d reușite, %d erori.\n", ok, bad)
}

type job struct{ src, dst string }

// planOutputs stabilește numele de ieșire dinainte, ca două surse care ar da
// același nume (ex: poza.gif și poza.png → poza.png) să nu se suprascrie între ele.
func planOutputs(files []string, cfg config) []job {
	count := map[string]int{}
	for _, f := range files {
		count[strings.ToLower(outName(f, cfg.format))]++
	}
	jobs := make([]job, 0, len(files))
	for _, f := range files {
		name := outName(f, cfg.format)
		if count[strings.ToLower(name)] > 1 {
			base := strings.TrimSuffix(filepath.Base(f), filepath.Ext(f))
			srcExt := strings.TrimPrefix(strings.ToLower(filepath.Ext(f)), ".")
			name = base + "-" + srcExt + filepath.Ext(name)
		}
		jobs = append(jobs, job{src: f, dst: filepath.Join(cfg.outDir, name)})
	}
	return jobs
}

func outName(path, format string) string {
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)) + outExt(path, format)
}

func process(j job, cfg config) error {
	path := j.src
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	src, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("decodare: %w", err)
	}
	if format == "jpeg" {
		// pozele de pe telefon sunt adesea "rotite" doar prin EXIF
		src = applyOrientation(src, jpegOrientation(data))
	}

	var out *image.NRGBA
	switch cfg.mode {
	case "fit":
		out = fit(src, cfg.maxW, cfg.maxH, cfg.noUp)
	case "pad":
		out = pad(src, cfg)
	case "fill":
		out = fill(src, cfg.maxW, cfg.maxH)
	}

	if err := save(j.dst, out, strings.ToLower(filepath.Ext(j.dst)), cfg); err != nil {
		return err
	}

	b := src.Bounds()
	fmt.Printf("✓ %s  %dx%d → %s %dx%d\n", filepath.Base(path), b.Dx(), b.Dy(), filepath.Base(j.dst), out.Bounds().Dx(), out.Bounds().Dy())
	return nil
}

// ---------- resize ----------

func fitSize(sw, sh, maxW, maxH int, noUp bool) (int, int) {
	scale := math.Inf(1)
	if maxW > 0 {
		scale = float64(maxW) / float64(sw)
	}
	if maxH > 0 {
		scale = math.Min(scale, float64(maxH)/float64(sh))
	}
	if noUp && scale > 1 {
		scale = 1
	}
	w := max(1, int(math.Round(float64(sw)*scale)))
	h := max(1, int(math.Round(float64(sh)*scale)))
	if maxW > 0 && w > maxW {
		w = maxW
	}
	if maxH > 0 && h > maxH {
		h = maxH
	}
	return w, h
}

func fit(src image.Image, maxW, maxH int, noUp bool) *image.NRGBA {
	b := src.Bounds()
	w, h := fitSize(b.Dx(), b.Dy(), maxW, maxH, noUp)
	return resample(src, b, w, h)
}

func pad(src image.Image, cfg config) *image.NRGBA {
	fitted := fit(src, cfg.maxW, cfg.maxH, cfg.noUp)
	canvas := image.NewNRGBA(image.Rect(0, 0, cfg.maxW, cfg.maxH))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: cfg.bg}, image.Point{}, draw.Src)
	fb := fitted.Bounds()
	off := image.Pt((cfg.maxW-fb.Dx())/2, (cfg.maxH-fb.Dy())/2)
	draw.Draw(canvas, fb.Add(off), fitted, image.Point{}, draw.Over)
	return canvas
}

func fill(src image.Image, w, h int) *image.NRGBA {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	target := float64(w) / float64(h)
	crop := b
	if float64(sw)/float64(sh) > target {
		cw := max(1, int(math.Round(float64(sh)*target)))
		x0 := b.Min.X + (sw-cw)/2
		crop = image.Rect(x0, b.Min.Y, x0+cw, b.Max.Y)
	} else {
		ch := max(1, int(math.Round(float64(sw)/target)))
		y0 := b.Min.Y + (sh-ch)/2
		crop = image.Rect(b.Min.X, y0, b.Max.X, y0+ch)
	}
	return resample(src, crop, w, h)
}

func resample(src image.Image, sr image.Rectangle, w, h int) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	if sr.Dx() == w && sr.Dy() == h {
		draw.Draw(dst, dst.Bounds(), src, sr.Min, draw.Src)
	} else {
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, sr, draw.Src, nil)
	}
	return dst
}

// ---------- output ----------

func outExt(path, format string) string {
	switch format {
	case "jpg":
		return ".jpg"
	case "png":
		return ".png"
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".jpg" || ext == ".jpeg" || ext == ".png" {
		return ext
	}
	return ".png" // gif/webp → png (Go nu are encoder webp în stdlib)
}

func save(path string, img *image.NRGBA, ext string, cfg config) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	switch ext {
	case ".jpg", ".jpeg":
		bg := cfg.bg
		if bg.A < 255 {
			bg = color.NRGBA{255, 255, 255, 255}
		}
		flat := image.NewNRGBA(img.Bounds())
		draw.Draw(flat, flat.Bounds(), &image.Uniform{C: bg}, image.Point{}, draw.Src)
		draw.Draw(flat, flat.Bounds(), img, img.Bounds().Min, draw.Over)
		err = jpeg.Encode(f, flat, &jpeg.Options{Quality: cfg.quality})
	default:
		err = png.Encode(f, img)
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

func parseColor(s string) (color.NRGBA, error) {
	if strings.EqualFold(s, "transparent") {
		return color.NRGBA{}, nil
	}
	h := strings.TrimPrefix(s, "#")
	if len(h) != 6 {
		return color.NRGBA{}, fmt.Errorf("culoare invalidă %q (ex: ffffff, #000000, transparent)", s)
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return color.NRGBA{}, fmt.Errorf("culoare invalidă %q", s)
	}
	return color.NRGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255}, nil
}

// ---------- EXIF orientation ----------

func jpegOrientation(data []byte) int {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return 1
	}
	i := 2
	for i+4 <= len(data) {
		if data[i] != 0xFF {
			return 1
		}
		marker := data[i+1]
		if marker == 0xFF { // padding
			i++
			continue
		}
		if marker == 0xDA || marker == 0xD9 { // start of scan / end of image
			return 1
		}
		segLen := int(binary.BigEndian.Uint16(data[i+2:]))
		if segLen < 2 || i+2+segLen > len(data) {
			return 1
		}
		seg := data[i+4 : i+2+segLen]
		if marker == 0xE1 && len(seg) > 14 && string(seg[:6]) == "Exif\x00\x00" {
			return tiffOrientation(seg[6:])
		}
		i += 2 + segLen
	}
	return 1
}

func tiffOrientation(t []byte) int {
	if len(t) < 8 {
		return 1
	}
	var bo binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 1
	}
	off := int(bo.Uint32(t[4:]))
	if off < 8 || off+2 > len(t) {
		return 1
	}
	n := int(bo.Uint16(t[off:]))
	for k := 0; k < n; k++ {
		e := off + 2 + k*12
		if e+12 > len(t) {
			return 1
		}
		if bo.Uint16(t[e:]) == 0x0112 {
			if o := int(bo.Uint16(t[e+8:])); o >= 1 && o <= 8 {
				return o
			}
			return 1
		}
	}
	return 1
}

func applyOrientation(src image.Image, o int) image.Image {
	if o <= 1 || o > 8 {
		return src
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	s := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.Draw(s, s.Bounds(), src, b.Min, draw.Src)

	dw, dh := w, h
	if o >= 5 {
		dw, dh = h, w
	}
	d := image.NewNRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var dx, dy int
			switch o {
			case 2:
				dx, dy = w-1-x, y
			case 3:
				dx, dy = w-1-x, h-1-y
			case 4:
				dx, dy = x, h-1-y
			case 5:
				dx, dy = y, x
			case 6:
				dx, dy = h-1-y, x
			case 7:
				dx, dy = h-1-y, w-1-x
			case 8:
				dx, dy = y, w-1-x
			}
			si, di := s.PixOffset(x, y), d.PixOffset(dx, dy)
			copy(d.Pix[di:di+4], s.Pix[si:si+4])
		}
	}
	return d
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "eroare: "+format+"\n", args...)
	flag.Usage()
	os.Exit(1)
}
