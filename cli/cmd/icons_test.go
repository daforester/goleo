package cmd

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// makeSourcePNG builds a size×size test image (a colored gradient with a
// semi-transparent corner) and writes it to path.
func makeSourcePNG(t *testing.T, path string, size int) image.Image {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			a := uint8(255)
			if x < size/4 && y < size/4 {
				a = 128
			}
			img.SetRGBA(x, y, color.RGBA{
				R: uint8(x * 255 / size),
				G: uint8(y * 255 / size),
				B: 128,
				A: a,
			})
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	return img
}

func TestResizeDimensions(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 512, 512))
	out := resizeRGBA(src, 48, 48)
	if out.Bounds().Dx() != 48 || out.Bounds().Dy() != 48 {
		t.Fatalf("got %v, want 48x48", out.Bounds())
	}
}

func TestPngToICOMulti(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "icon.png")
	makeSourcePNG(t, src, 512)

	ico, cleanup, err := pngToICOMulti(src)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	data, err := os.ReadFile(ico)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 6 {
		t.Fatal("ico too small")
	}
	le := binary.LittleEndian
	if le.Uint16(data[0:]) != 0 || le.Uint16(data[2:]) != 1 {
		t.Fatalf("bad ICONDIR header")
	}
	count := le.Uint16(data[4:])
	if count < 2 {
		t.Fatalf("want multiple ico entries, got %d", count)
	}
	// First entry: read its offset+size and confirm the payload is a valid PNG.
	off := le.Uint32(data[6+12:])
	sz := le.Uint32(data[6+8:])
	if _, err := png.DecodeConfig(bytes.NewReader(data[off : off+sz])); err != nil {
		t.Fatalf("first ico entry is not a valid PNG: %v", err)
	}
}

func TestWriteICNS(t *testing.T) {
	dir := t.TempDir()
	src := makeSourcePNG(t, filepath.Join(dir, "icon.png"), 512)
	out := filepath.Join(dir, "icon.icns")
	if err := writeICNS(src, out); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(data[0:4]) != "icns" {
		t.Fatalf("missing icns magic")
	}
	total := binary.BigEndian.Uint32(data[4:8])
	if int(total) != len(data) {
		t.Fatalf("icns length header %d != file size %d", total, len(data))
	}
	// First chunk should be ic10 with a decodable PNG payload.
	if string(data[8:12]) != "ic10" {
		t.Fatalf("first chunk = %q, want ic10", data[8:12])
	}
	clen := binary.BigEndian.Uint32(data[12:16])
	if _, err := png.DecodeConfig(bytes.NewReader(data[16 : 8+clen])); err != nil {
		t.Fatalf("ic10 payload not a PNG: %v", err)
	}
}

func TestGenerateAndroidIcons(t *testing.T) {
	dir := t.TempDir()
	src := makeSourcePNG(t, filepath.Join(dir, "icon.png"), 512)
	resDir := filepath.Join(dir, "res")
	if err := generateAndroidIcons(src, resDir); err != nil {
		t.Fatal(err)
	}
	for bucket, size := range androidDensities {
		for _, name := range []string{"ic_launcher.png", "ic_launcher_round.png"} {
			p := filepath.Join(resDir, bucket, name)
			f, err := os.Open(p)
			if err != nil {
				t.Fatalf("missing %s: %v", p, err)
			}
			cfg, err := png.DecodeConfig(f)
			f.Close()
			if err != nil {
				t.Fatalf("%s not a PNG: %v", p, err)
			}
			if cfg.Width != size || cfg.Height != size {
				t.Fatalf("%s is %dx%d, want %d", p, cfg.Width, cfg.Height, size)
			}
		}
	}
}

func TestGenerateIOSAppIcon(t *testing.T) {
	dir := t.TempDir()
	src := makeSourcePNG(t, filepath.Join(dir, "icon.png"), 512)
	assets := filepath.Join(dir, "Assets.xcassets")
	if err := generateIOSAppIcon(src, assets); err != nil {
		t.Fatal(err)
	}
	setDir := filepath.Join(assets, "AppIcon.appiconset")
	if _, err := os.Stat(filepath.Join(setDir, "Contents.json")); err != nil {
		t.Fatalf("missing Contents.json: %v", err)
	}
	f, err := os.Open(filepath.Join(setDir, "icon-1024.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cfg, err := png.DecodeConfig(f)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Width != 1024 || cfg.Height != 1024 {
		t.Fatalf("icon-1024 is %dx%d, want 1024", cfg.Width, cfg.Height)
	}
}
