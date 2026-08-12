package cmd

import (
	"strings"
	"testing"
)

// The iOS analogue of TestDevManifestCoversEveryFeaturePermission.
//
// featureRegistry declares IOSUsageDescs for eight features and **nothing reads them** — the
// Info.plist template hardcodes its own strings. So the registry looks like the source of
// truth for iOS purpose strings and is not one, which is the same shape as the Android
// dev/release manifest split: a declared source of truth that one path ignores.
//
// It matters more on iOS than the Android equivalent did. A missing purpose string does not
// deny the request — iOS **terminates the app** the first time it touches the protected
// resource. So a feature shipped without its string is a crash, not a degraded page.
//
// This does not auto-inject anything (wiring IOSUsageDescs into the plist is a bigger change,
// tracked in docs/roadmap.md). It fails when the two disagree, so the decision is forced at
// the point a feature is added rather than discovered on a device.
var iosPlistMayOmit = map[string]string{
	"NSPhotoLibraryUsageDescription": "the iOS dialogs use UIDocumentPickerViewController, which " +
		"reads no photo library; adding it would declare a purpose the app never exercises",
	"NSDocumentsFolderUsageDescription": "a macOS key — it has no effect on iOS at all",
	"NSMotionUsageDescription": "gates CMPedometer / CMMotionActivityManager. GoleoSensors uses " +
		"raw accelerometer/gyroscope/magnetometer via CMMotionManager, which do not require it — " +
		"all three were verified working on an iPhone 17 Pro Max without this key",
	// The next two are landmines rather than exemptions: read them before implementing
	// either feature on iOS.
	"NSBluetoothAlwaysUsageDescription": "iOS has no CoreBluetooth path — AppDelegate registers " +
		"no BLE provider and the demo reports ios:'no'. ADD THIS TO Info.plist IN THE SAME " +
		"CHANGE that introduces one, or iOS will terminate the app on first CBCentralManager use",
	"NFCReaderUsageDescription": "iOS has no CoreNFC path — AppDelegate registers no NFC " +
		"provider. ADD THIS TO Info.plist IN THE SAME CHANGE that introduces one. Note it needs " +
		"the com.apple.developer.nfc.readersession.formats ENTITLEMENT too, which goleo does " +
		"not generate at all — the purpose string alone is not enough, and the key has no NS " +
		"prefix, so a search for NS*UsageDescription misses it (this test did not)",
}

func TestIOSUsageDescriptionsReachTheInfoPlist(t *testing.T) {
	plist, err := mobileTemplates.ReadFile("templates/ios/App/Info.plist")
	if err != nil {
		t.Fatal(err)
	}
	info := string(plist)

	for _, f := range featureRegistry {
		for key := range f.IOSUsageDescs {
			if _, ok := iosPlistMayOmit[key]; ok {
				continue
			}
			if !strings.Contains(info, key) {
				t.Errorf("the %s feature declares %s in featureRegistry, but the iOS "+
					"Info.plist template does not contain it. Nothing injects IOSUsageDescs, "+
					"so the app ships without the string and iOS TERMINATES it on first use "+
					"of the resource. Add it to templates/ios/App/Info.plist, or to "+
					"iosPlistMayOmit with the reason it is not needed.", f.Name, key)
			}
		}
	}
}

// The three strings the shipped app actually relies on. These are not "may omit" candidates:
// camera and microphone are exercised by the WebView's getUserMedia and location by
// navigator.geolocation, all three on a path the demo drives today.
func TestInfoPlistKeepsTheUsageDescriptionsInUse(t *testing.T) {
	plist, err := mobileTemplates.ReadFile("templates/ios/App/Info.plist")
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{
		"NSCameraUsageDescription",
		"NSMicrophoneUsageDescription",
		"NSLocationWhenInUseUsageDescription",
	} {
		if !strings.Contains(string(plist), key) {
			t.Errorf("Info.plist no longer declares %s — the WebView grants the matching "+
				"capture/location request, and iOS terminates an app that takes it with no "+
				"purpose string", key)
		}
	}
}
