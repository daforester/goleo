//go:build linux && !android

package battery

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const powerSupplyDir = "/sys/class/power_supply"

func platformGetBatteryInfo() (*BatteryInfo, error) {
	entries, err := os.ReadDir(powerSupplyDir)
	if err != nil {
		return nil, errors.New("battery: no power supply information available")
	}
	for _, e := range entries {
		dir := filepath.Join(powerSupplyDir, e.Name())
		if readSysfs(dir, "type") != "Battery" {
			continue
		}
		info := &BatteryInfo{ChargingTime: -1, DischargingTime: -1}

		capacity := readSysfs(dir, "capacity")
		pct, err := strconv.Atoi(capacity)
		if err != nil {
			continue
		}
		info.Level = float64(pct) / 100

		status := readSysfs(dir, "status")
		info.Charging = status == "Charging" || status == "Full"
		if status == "Full" {
			info.ChargingTime = 0
		}

		// Estimate remaining time from charge/current or energy/power pairs.
		if secs, ok := remainingSeconds(dir, info.Charging); ok {
			if info.Charging {
				info.ChargingTime = secs
			} else {
				info.DischargingTime = secs
			}
		}
		return info, nil
	}
	return nil, errors.New("battery: no system battery present")
}

func remainingSeconds(dir string, charging bool) (float64, bool) {
	now := readSysfsInt(dir, "charge_now")
	full := readSysfsInt(dir, "charge_full")
	rate := readSysfsInt(dir, "current_now")
	if now < 0 || rate <= 0 {
		now = readSysfsInt(dir, "energy_now")
		full = readSysfsInt(dir, "energy_full")
		rate = readSysfsInt(dir, "power_now")
	}
	if now < 0 || rate <= 0 {
		return 0, false
	}
	if charging {
		if full < 0 {
			return 0, false
		}
		return float64(full-now) / float64(rate) * 3600, true
	}
	return float64(now) / float64(rate) * 3600, true
}

func readSysfs(dir, name string) string {
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func readSysfsInt(dir, name string) int64 {
	v, err := strconv.ParseInt(readSysfs(dir, name), 10, 64)
	if err != nil {
		return -1
	}
	return v
}
