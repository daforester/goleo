//go:build !mobilebuild && !js && !darwin

package runtime

// Desktop platforms (Windows/macOS/Linux) can host native windows and a system
// tray. The mobilebuild / js counterpart (capabilities_unsupported.go) reports
// these as unavailable so shared app code degrades gracefully.
const (
	platformWindowing = true
	platformTray      = true
)
