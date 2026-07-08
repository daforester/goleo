//go:build darwin && !ios

package geolocation

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// CoreLocation needs an Objective-C delegate, which pure Go cannot provide
// without cgo. If the CoreLocationCLI helper is installed
// (brew install corelocationcli) it is used; otherwise the frontend falls
// back to navigator.geolocation in the WKWebView.
func platformGetCurrentPosition(opts PositionOptions) (*Position, error) {
	bin, err := exec.LookPath("CoreLocationCLI")
	if err != nil {
		return nil, fmt.Errorf("geolocation: %w on macOS without CoreLocationCLI (brew install corelocationcli)", errors.ErrUnsupported)
	}

	timeout := 30 * time.Second
	if opts.Timeout > 0 {
		timeout = time.Duration(opts.Timeout) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, bin, "-once", "-format", "%latitude|%longitude|%h_accuracy").Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("geolocation: timed out after %s", timeout)
		}
		return nil, fmt.Errorf("geolocation: CoreLocationCLI failed: %w", err)
	}

	parts := strings.Split(strings.TrimSpace(string(out)), "|")
	if len(parts) < 2 {
		return nil, fmt.Errorf("geolocation: unexpected output %q", strings.TrimSpace(string(out)))
	}
	lat, err1 := strconv.ParseFloat(parts[0], 64)
	lon, err2 := strconv.ParseFloat(parts[1], 64)
	if err1 != nil || err2 != nil {
		return nil, fmt.Errorf("geolocation: could not parse coordinates from %q", strings.TrimSpace(string(out)))
	}
	pos := &Position{Latitude: lat, Longitude: lon}
	if len(parts) >= 3 {
		pos.Accuracy, _ = strconv.ParseFloat(parts[2], 64)
	}
	return pos, nil
}
