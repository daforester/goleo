//go:build windows

package battery

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"
)

var (
	kernel32                 = syscall.NewLazyDLL("kernel32.dll")
	procGetSystemPowerStatus = kernel32.NewProc("GetSystemPowerStatus")
)

// https://learn.microsoft.com/en-us/windows/win32/api/winbase/ns-winbase-system_power_status
type systemPowerStatus struct {
	ACLineStatus        byte
	BatteryFlag         byte
	BatteryLifePercent  byte
	SystemStatusFlag    byte
	BatteryLifeTime     uint32
	BatteryFullLifeTime uint32
}

const (
	batteryFlagCharging  = 8
	batteryFlagNoBattery = 128
	valueUnknown         = 0xFFFFFFFF
)

func platformGetBatteryInfo() (*BatteryInfo, error) {
	var s systemPowerStatus
	r, _, callErr := procGetSystemPowerStatus.Call(uintptr(unsafe.Pointer(&s)))
	if r == 0 {
		return nil, fmt.Errorf("battery: GetSystemPowerStatus failed: %w", callErr)
	}
	if s.BatteryFlag&batteryFlagNoBattery != 0 {
		return nil, errors.New("battery: no system battery present")
	}

	info := &BatteryInfo{
		Charging:        s.BatteryFlag&batteryFlagCharging != 0,
		ChargingTime:    -1,
		DischargingTime: -1,
	}
	if s.BatteryLifePercent <= 100 {
		info.Level = float64(s.BatteryLifePercent) / 100
	} else {
		info.Level = -1 // 255 = unknown
	}
	if !info.Charging && s.BatteryLifeTime != valueUnknown {
		info.DischargingTime = float64(s.BatteryLifeTime)
	}
	if info.Charging && info.Level >= 1 {
		info.ChargingTime = 0
	}
	return info, nil
}
