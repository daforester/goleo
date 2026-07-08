//go:build windows

package geolocation

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Uses the WinRT Geolocator through Windows PowerShell 5.1 (the WinRT
// projection is not available in pwsh 7+). Requires Windows location
// services to be enabled; access status is checked first so a clear error
// reaches the frontend, which then falls back to navigator.geolocation.
const geolocatorScript = `
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Runtime.WindowsRuntime
$null = [Windows.Devices.Geolocation.Geolocator,Windows.Devices.Geolocation,ContentType=WindowsRuntime]
$asTaskGeneric = ([System.WindowsRuntimeSystemExtensions].GetMethods() | Where-Object {
    $_.Name -eq 'AsTask' -and $_.GetParameters().Count -eq 1 -and
    $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncOperation` + "`" + `1' })[0]
function Await($op, $resultType) {
    $task = $asTaskGeneric.MakeGenericMethod($resultType).Invoke($null, @($op))
    $null = $task.Wait(-1)
    $task.Result
}
$access = Await ([Windows.Devices.Geolocation.Geolocator]::RequestAccessAsync()) ([Windows.Devices.Geolocation.GeolocationAccessStatus])
if ($access -ne 'Allowed') { throw "location access is $access (enable it in Settings > Privacy > Location)" }
$geo = New-Object Windows.Devices.Geolocation.Geolocator
$geo.DesiredAccuracy = '%s'
$pos = Await ($geo.GetGeopositionAsync()) ([Windows.Devices.Geolocation.Geoposition])
$inv = [System.Globalization.CultureInfo]::InvariantCulture
$c = $pos.Coordinate
Write-Output ($c.Point.Position.Latitude.ToString($inv) + '|' + $c.Point.Position.Longitude.ToString($inv) + '|' + $c.Accuracy.ToString($inv))
`

func platformGetCurrentPosition(opts PositionOptions) (*Position, error) {
	accuracy := "Default"
	if opts.EnableHighAccuracy {
		accuracy = "High"
	}
	timeout := 30 * time.Second
	if opts.Timeout > 0 {
		timeout = time.Duration(opts.Timeout) * time.Millisecond
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	script := fmt.Sprintf(geolocatorScript, accuracy)
	out, err := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("geolocation: timed out after %s", timeout)
		}
		detail := ""
		if ee, ok := err.(*exec.ExitError); ok {
			detail = ": " + strings.TrimSpace(string(ee.Stderr))
		}
		return nil, fmt.Errorf("geolocation: %w%s", err, detail)
	}

	parts := strings.Split(strings.TrimSpace(string(out)), "|")
	if len(parts) != 3 {
		return nil, fmt.Errorf("geolocation: unexpected output %q", strings.TrimSpace(string(out)))
	}
	lat, err1 := strconv.ParseFloat(parts[0], 64)
	lon, err2 := strconv.ParseFloat(parts[1], 64)
	acc, _ := strconv.ParseFloat(parts[2], 64)
	if err1 != nil || err2 != nil {
		return nil, fmt.Errorf("geolocation: could not parse coordinates from %q", strings.TrimSpace(string(out)))
	}
	return &Position{Latitude: lat, Longitude: lon, Accuracy: acc}, nil
}
