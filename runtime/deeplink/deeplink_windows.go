//go:build windows

package deeplink

import "golang.org/x/sys/windows/registry"

func platformRegister(scheme, appName, exePath string) error {
	base := `Software\Classes\` + scheme
	k, _, err := registry.CreateKey(registry.CURRENT_USER, base, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.SetStringValue("", "URL:"+appName); err != nil {
		return err
	}
	// The empty-named "URL Protocol" value marks this as a URL scheme handler.
	if err := k.SetStringValue("URL Protocol", ""); err != nil {
		return err
	}

	cmd, _, err := registry.CreateKey(registry.CURRENT_USER, base+`\shell\open\command`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer cmd.Close()
	return cmd.SetStringValue("", `"`+exePath+`" "%1"`)
}
