package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// goleoJSON is the typed representation of a project's goleo.json.
//
// It replaces four independent `map[string]any` parsers (loadBundleConfig,
// loadMobileConfig, loadFrontendConfig and emulate.go's extractPackageName), each
// of which swallowed parse errors and returned defaults. That combination meant a
// single trailing comma silently produced an APK with the wrong applicationId and
// a .dmg with the wrong CFBundleIdentifier and version — a misbuild with no
// diagnostic. It also let three documented keys rot unread for lack of one place
// that owned the schema (mobile.android.min_sdk, mobile.ios.deployment_target and
// mobile.ios.bundle_identifier; the iOS bundle id was being derived from the
// *Android* package name).
//
// Unknown keys are deliberately tolerated (no DisallowUnknownFields): configs
// carrying extra keys are legitimate, and rejecting them would break projects for
// no safety gain. A malformed *document*, or a key of the wrong type, is an error
// — see checkGoleoJSON.
type goleoJSON struct {
	Version  string          `json:"version"`
	AppName  string          `json:"app_name"`
	Frontend frontendSection `json:"frontend"`
	Bundle   bundleSection   `json:"bundle"`
	Mobile   mobileSection   `json:"mobile"`
	Windows  windowsSection  `json:"windows"`
}

type frontendSection struct {
	Directory    string `json:"directory"`
	BuildCommand string `json:"build_command"`
	DevCommand   string `json:"dev_command"`
	DevPort      int    `json:"dev_port"`
	DistDir      string `json:"dist_dir"`
}

type bundleSection struct {
	Identifier    string `json:"identifier"`
	Publisher     string `json:"publisher"`
	Description   string `json:"description"`
	Copyright     string `json:"copyright"`
	Category      string `json:"category"`
	Homepage      string `json:"homepage"`
	Icon          string `json:"icon"`
	IconICO       string `json:"icon_ico"`
	IconICNS      string `json:"icon_icns"`
	IconPNG       string `json:"icon_png"`
	UpdateURLBase string `json:"update_url_base"`
	ReleaseNotes  string `json:"release_notes"`
	URLScheme     string `json:"url_scheme"`
}

type mobileSection struct {
	Android androidSection `json:"android"`
	IOS     iosSection     `json:"ios"`
}

type androidSection struct {
	PackageName string `json:"package_name"`
	MinSDK      int    `json:"min_sdk"`
	TargetSDK   int    `json:"target_sdk"`
	// VersionCode overrides the value derived from Version. Play requires a
	// monotonically increasing integer per upload.
	VersionCode int `json:"version_code"`
	// ExtraPermissions are added to the generated manifest verbatim, for
	// permissions goleo cannot infer from the features the app compiles in.
	ExtraPermissions []string `json:"extra_permissions"`
}

type iosSection struct {
	BundleIdentifier string `json:"bundle_identifier"`
	DeploymentTarget string `json:"deployment_target"`
	// DevelopmentTeam is the 10-character Apple Developer Team ID that signs a
	// DEVICE build. Without it xcodebuild stops at "Signing for \"App\" requires a
	// development team", which is why `goleo build ios` could only ever produce a
	// Simulator app from the CLI. Not needed for --simulator, which does not sign.
	DevelopmentTeam string `json:"development_team"`
}

// windowsSection is reserved for MSIX identity (Microsoft Store), which must
// match Partner Center exactly.
type windowsSection struct {
	MSIX msixSection `json:"msix"`
}

type msixSection struct {
	IdentityName         string `json:"identity_name"`
	Publisher            string `json:"publisher"`
	PublisherDisplayName string `json:"publisher_display_name"`
}

// parseGoleoJSON reads and strictly parses goleo.json. A missing file is not an
// error (found=false) — several commands run fine without one — but a file that
// exists and does not parse is.
func parseGoleoJSON(projectDir string) (cfg goleoJSON, found bool, err error) {
	path := filepath.Join(projectDir, "goleo.json")
	data, rerr := os.ReadFile(path)
	if rerr != nil {
		if os.IsNotExist(rerr) {
			return cfg, false, nil
		}
		return cfg, false, fmt.Errorf("reading %s: %w", path, rerr)
	}
	if uerr := json.Unmarshal(data, &cfg); uerr != nil {
		return cfg, true, fmt.Errorf("%s is not valid JSON: %w", path, uerr)
	}
	return cfg, true, nil
}

// loadGoleoJSON is the tolerant accessor used by the config adapters below. It
// discards parse errors on purpose: checkGoleoJSON already surfaced them at
// command entry, and these adapters are called from chained expressions
// (loadFrontendConfig(".").DistDir) whose signatures have no error to return.
// Anything reached without checkGoleoJSON degrades to defaults exactly as before.
func loadGoleoJSON(projectDir string) goleoJSON {
	cfg, _, _ := parseGoleoJSON(projectDir)
	return cfg
}
