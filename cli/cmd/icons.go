package cmd

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

// icons.go — pure-Go app-icon generation. From a single source PNG (recommended
// 1024×1024) it derives every per-platform artifact: multi-size Windows .ico,
// macOS .icns, Linux hicolor PNGs, Android mipmap launcher icons (square + round),
// and an iOS AppIcon.appiconset. No external tooling, no cgo — just image/draw.

// resolveSourceIcon returns the source icon PNG path (the explicit bundle.icon)
// if it exists, plus whether it was found. Paths are resolved relative to the
// current working directory (the project dir during a build).
func resolveSourceIcon(cfg bundleConfig) (string, bool) {
	if cfg.Icon == "" {
		return "", false
	}
	if _, err := os.Stat(cfg.Icon); err != nil {
		return "", false
	}
	return cfg.Icon, true
}

// mobileIconSource loads the project's source icon (bundle.icon in goleo.json)
// for a mobile build. Returns (nil, false) when none is configured or it can't be
// decoded, in which case the build keeps the platform's default launcher icon.
func mobileIconSource() (image.Image, bool) {
	path, ok := resolveSourceIcon(loadBundleConfig("."))
	if !ok {
		return nil, false
	}
	img, err := loadPNG(path)
	if err != nil {
		return nil, false
	}
	return img, true
}

// loadPNG decodes a PNG file into an image.Image.
func loadPNG(path string) (image.Image, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%s is not a valid PNG: %w", path, err)
	}
	return img, nil
}

// resizeRGBA resamples src to w×h. For downscaling (the common case — a large
// master shrunk to icon sizes) it area-averages the covered source box, which
// gives clean, alias-free icons; for upscaling the box collapses to a single
// sample (nearest). Averaging is done in Go's alpha-premultiplied space, which is
// what color.RGBA already stores, so no separate premultiply step is needed.
func resizeRGBA(src image.Image, w, h int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	if sw == 0 || sh == 0 {
		return dst
	}
	xr := float64(sw) / float64(w)
	yr := float64(sh) / float64(h)
	for dy := 0; dy < h; dy++ {
		sy0 := int(float64(dy) * yr)
		sy1 := int(float64(dy+1) * yr)
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		if sy1 > sh {
			sy1 = sh
		}
		for dx := 0; dx < w; dx++ {
			sx0 := int(float64(dx) * xr)
			sx1 := int(float64(dx+1) * xr)
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			if sx1 > sw {
				sx1 = sw
			}
			var r, g, b, a, n uint64
			for yy := sy0; yy < sy1; yy++ {
				for xx := sx0; xx < sx1; xx++ {
					cr, cg, cb, ca := src.At(sb.Min.X+xx, sb.Min.Y+yy).RGBA()
					r += uint64(cr)
					g += uint64(cg)
					b += uint64(cb)
					a += uint64(ca)
					n++
				}
			}
			if n == 0 {
				n = 1
			}
			// 16-bit premultiplied average → 8-bit premultiplied (color.RGBA).
			dst.SetRGBA(dx, dy, color.RGBA{
				R: uint8((r / n) >> 8),
				G: uint8((g / n) >> 8),
				B: uint8((b / n) >> 8),
				A: uint8((a / n) >> 8),
			})
		}
	}
	return dst
}

// circularMask returns a copy of img with pixels outside the inscribed circle
// made transparent — used for Android's round launcher icon.
func circularMask(img *image.RGBA) *image.RGBA {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	out := image.NewRGBA(b)
	cx, cy := float64(w)/2, float64(h)/2
	rad := cx
	if cy < rad {
		rad = cy
	}
	r2 := rad * rad
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx, dy := float64(x)+0.5-cx, float64(y)+0.5-cy
			if dx*dx+dy*dy <= r2 {
				out.Set(x, y, img.At(b.Min.X+x, b.Min.Y+y))
			}
		}
	}
	return out
}

// encodePNGBytes encodes an image to PNG bytes.
func encodePNGBytes(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// writeResizedPNG writes a size×size PNG derived from src to path.
func writeResizedPNG(src image.Image, size int, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := encodePNGBytes(resizeRGBA(src, size, size))
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// --- Windows .ico (multi-size, PNG-compressed entries) ---

// pngToICOMulti builds a multi-resolution .ico (16/32/48/64/128/256, capped at the
// source's largest dimension) from a source PNG, written to a temp file. Vista+
// reads PNG-encoded icon directory entries directly, so every entry is a PNG.
func pngToICOMulti(pngPath string) (string, func(), error) {
	src, err := loadPNG(pngPath)
	if err != nil {
		return "", nil, err
	}
	maxDim := src.Bounds().Dx()
	if src.Bounds().Dy() > maxDim {
		maxDim = src.Bounds().Dy()
	}
	var sizes []int
	for _, s := range []int{16, 32, 48, 64, 128, 256} {
		if s <= maxDim || len(sizes) == 0 {
			sizes = append(sizes, s)
		}
	}

	type entry struct {
		size int
		data []byte
	}
	var entries []entry
	for _, s := range sizes {
		data, err := encodePNGBytes(resizeRGBA(src, s, s))
		if err != nil {
			return "", nil, err
		}
		entries = append(entries, entry{s, data})
	}

	buf := &bytes.Buffer{}
	le := binary.LittleEndian
	_ = binary.Write(buf, le, uint16(0))              // reserved
	_ = binary.Write(buf, le, uint16(1))              // type: icon
	_ = binary.Write(buf, le, uint16(len(entries)))   // count
	offset := 6 + 16*len(entries)                     // dir + entries
	dimByte := func(n int) byte {
		if n >= 256 {
			return 0 // 0 encodes 256 in an ICONDIRENTRY
		}
		return byte(n)
	}
	for _, e := range entries {
		buf.WriteByte(dimByte(e.size)) // width
		buf.WriteByte(dimByte(e.size)) // height
		buf.WriteByte(0)               // palette count
		buf.WriteByte(0)               // reserved
		_ = binary.Write(buf, le, uint16(1))             // color planes
		_ = binary.Write(buf, le, uint16(32))            // bpp
		_ = binary.Write(buf, le, uint32(len(e.data)))   // data size
		_ = binary.Write(buf, le, uint32(offset))        // data offset
		offset += len(e.data)
	}
	for _, e := range entries {
		buf.Write(e.data)
	}

	tmp, err := os.CreateTemp("", "goleo-icon-*.ico")
	if err != nil {
		return "", nil, err
	}
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", nil, err
	}
	tmp.Close()
	return tmp.Name(), func() { os.Remove(tmp.Name()) }, nil
}

// --- macOS .icns (PNG-encoded entries) ---

// icnsEntry maps an ICNS OSType to the pixel size of its PNG payload.
type icnsEntry struct {
	osType string
	size   int
}

// Modern PNG-based ICNS types. Duplicate pixel sizes (e.g. ic08/ic13 = 256) are
// intentional — they are distinct 1x/2x designations macOS selects between.
var icnsEntries = []icnsEntry{
	{"ic10", 1024}, // 512@2x
	{"ic09", 512},
	{"ic14", 512}, // 256@2x
	{"ic08", 256},
	{"ic13", 256}, // 128@2x
	{"ic07", 128},
	{"ic12", 64}, // 32@2x
	{"ic11", 32}, // 16@2x
}

// writeICNS assembles a macOS .icns from the source image at path.
func writeICNS(src image.Image, path string) error {
	var body bytes.Buffer
	for _, e := range icnsEntries {
		data, err := encodePNGBytes(resizeRGBA(src, e.size, e.size))
		if err != nil {
			return err
		}
		body.WriteString(e.osType)
		_ = binary.Write(&body, binary.BigEndian, uint32(len(data)+8))
		body.Write(data)
	}
	var out bytes.Buffer
	out.WriteString("icns")
	_ = binary.Write(&out, binary.BigEndian, uint32(body.Len()+8))
	out.Write(body.Bytes())
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, out.Bytes(), 0o644)
}

// --- Android launcher icons ---

// androidDensities maps a mipmap density bucket to the launcher-icon pixel size.
var androidDensities = map[string]int{
	"mipmap-mdpi":    48,
	"mipmap-hdpi":    72,
	"mipmap-xhdpi":   96,
	"mipmap-xxhdpi":  144,
	"mipmap-xxxhdpi": 192,
}

// generateAndroidIcons writes ic_launcher.png (square) and ic_launcher_round.png
// (circular) at every density under resDir (…/app/src/main/res).
func generateAndroidIcons(src image.Image, resDir string) error {
	for bucket, size := range androidDensities {
		dir := filepath.Join(resDir, bucket)
		square := resizeRGBA(src, size, size)
		sq, err := encodePNGBytes(square)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "ic_launcher.png"), sq, 0o644); err != nil {
			return err
		}
		rd, err := encodePNGBytes(circularMask(square))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "ic_launcher_round.png"), rd, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// --- iOS AppIcon.appiconset ---

// generateIOSAppIcon writes a single-size (1024×1024 universal) AppIcon set,
// which is all Xcode 14+ requires; it derives the smaller variants at build time.
func generateIOSAppIcon(src image.Image, assetsDir string) error {
	setDir := filepath.Join(assetsDir, "AppIcon.appiconset")
	if err := os.MkdirAll(setDir, 0o755); err != nil {
		return err
	}
	if err := writeResizedPNG(src, 1024, filepath.Join(setDir, "icon-1024.png")); err != nil {
		return err
	}
	contents := `{
  "images" : [
    {
      "filename" : "icon-1024.png",
      "idiom" : "universal",
      "platform" : "ios",
      "size" : "1024x1024"
    }
  ],
  "info" : {
    "author" : "goleo",
    "version" : 1
  }
}
`
	return os.WriteFile(filepath.Join(setDir, "Contents.json"), []byte(contents), 0o644)
}
