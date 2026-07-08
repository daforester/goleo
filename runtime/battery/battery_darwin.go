//go:build darwin && !ios

package battery

import (
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
)

// Example pmset output:
//
//	Now drawing from 'Battery Power'
//	 -InternalBattery-0 (id=1234567)	85%; discharging; 4:32 remaining present: true
var pmsetRe = regexp.MustCompile(`(\d+)%;\s*(\w[\w ]*?);(?:\s*(\d+):(\d+))?`)

func platformGetBatteryInfo() (*BatteryInfo, error) {
	out, err := exec.Command("pmset", "-g", "batt").Output()
	if err != nil {
		return nil, fmt.Errorf("battery: pmset failed: %w", err)
	}
	m := pmsetRe.FindStringSubmatch(string(out))
	if m == nil {
		return nil, errors.New("battery: no system battery present")
	}

	pct, _ := strconv.Atoi(m[1])
	info := &BatteryInfo{
		Level:           float64(pct) / 100,
		ChargingTime:    -1,
		DischargingTime: -1,
	}
	switch m[2] {
	case "charging":
		info.Charging = true
	case "charged", "finishing charge":
		info.Charging = true
		info.ChargingTime = 0
	}
	if m[3] != "" {
		h, _ := strconv.Atoi(m[3])
		min, _ := strconv.Atoi(m[4])
		secs := float64(h*3600 + min*60)
		if info.Charging {
			info.ChargingTime = secs
		} else {
			info.DischargingTime = secs
		}
	}
	return info, nil
}
