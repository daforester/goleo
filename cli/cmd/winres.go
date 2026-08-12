package cmd

import (
	"fmt"
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
			return pngToICOMulti(cfg.Icon) // multi-size .ico from the source PNG
		}
	}
	return "", nil, nil
}
