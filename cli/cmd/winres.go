package cmd

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image/png"
	"os"
	"path/filepath"

	"github.com/josephspurrier/goversioninfo"
)

// writeWindowsResource generates a `.syso` in pkgDir so `go build` embeds the app
// icon + version info (Details tab: product name, version, description, copyright,
// company) into the Windows executable. Returns a cleanup func that removes the
// generated files, or a nil cleanup if there is nothing to embed. Never fatal —
// a failure just means the exe keeps the default Go icon.
func writeWindowsResource(cfg bundleConfig, pkgDir, arch string) (func(), error) {
	icoPath, icoCleanup, err := resolveWindowsICO(cfg)
	if err != nil {
		return nil, err
	}

	desc := cfg.Description
	if desc == "" {
		desc = cfg.AppName
	}
	vi := &goversioninfo.VersionInfo{
		IconPath: icoPath,
		StringFileInfo: goversioninfo.StringFileInfo{
			ProductName:     cfg.AppName,
			FileDescription: desc,
			CompanyName:     cfg.Publisher,
			LegalCopyright:  cfg.Copyright,
			FileVersion:     cfg.Version,
			ProductVersion:  cfg.Version,
		},
	}
	if fv, err := goversioninfo.NewFileVersion(cfg.Version); err == nil {
		vi.FixedFileInfo.FileVersion = fv
		vi.FixedFileInfo.ProductVersion = fv
	}
	vi.Build() // populate the VS_VERSIONINFO structure
	vi.Walk()  // serialize it into vi.Buffer (read by WriteSyso)

	sysoPath := filepath.Join(pkgDir, fmt.Sprintf("zz_goleo_versioninfo_windows_%s.syso", arch))
	if err := vi.WriteSyso(sysoPath, arch); err != nil {
		if icoCleanup != nil {
			icoCleanup()
		}
		return nil, fmt.Errorf("write .syso: %w", err)
	}
	return func() {
		os.Remove(sysoPath)
		if icoCleanup != nil {
			icoCleanup()
		}
	}, nil
}

// resolveWindowsICO returns a path to a Windows .ico: an explicit bundle.icon_ico
// if set, otherwise one generated from the single bundle.icon PNG source. Returns
// "" (no icon, no error) when neither is available. The returned cleanup removes
// a generated temp .ico (nil for an explicit path).
func resolveWindowsICO(cfg bundleConfig) (string, func(), error) {
	if cfg.IconICO != "" {
		if _, err := os.Stat(cfg.IconICO); err == nil {
			return cfg.IconICO, nil, nil
		}
	}
	if cfg.Icon != "" {
		if _, err := os.Stat(cfg.Icon); err == nil {
			return pngToICO(cfg.Icon)
		}
	}
	return "", nil, nil
}

// pngToICO wraps a PNG in a single-entry .ico (Windows Vista+ reads PNG-encoded
// icon entries directly), writing it to a temp file. Returns the path + a cleanup.
func pngToICO(pngPath string) (string, func(), error) {
	data, err := os.ReadFile(pngPath)
	if err != nil {
		return "", nil, err
	}
	dim, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", nil, fmt.Errorf("%s is not a valid PNG: %w", pngPath, err)
	}
	dimByte := func(n int) byte {
		if n >= 256 {
			return 0 // 0 means 256 in an ICONDIRENTRY
		}
		return byte(n)
	}

	buf := &bytes.Buffer{}
	le := binary.LittleEndian
	// ICONDIR: reserved=0, type=1 (icon), count=1
	_ = binary.Write(buf, le, uint16(0))
	_ = binary.Write(buf, le, uint16(1))
	_ = binary.Write(buf, le, uint16(1))
	// ICONDIRENTRY (16 bytes)
	buf.WriteByte(dimByte(dim.Width))
	buf.WriteByte(dimByte(dim.Height))
	buf.WriteByte(0) // color count
	buf.WriteByte(0) // reserved
	_ = binary.Write(buf, le, uint16(1))            // color planes
	_ = binary.Write(buf, le, uint16(32))           // bits per pixel
	_ = binary.Write(buf, le, uint32(len(data)))    // size of PNG data
	_ = binary.Write(buf, le, uint32(6+16))         // offset to PNG data
	buf.Write(data)

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
