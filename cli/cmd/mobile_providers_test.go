package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A provider is only real when all three pieces exist: the runtime's SetXProvider, a
// gomobile bridge file exposing it to the shells, and BOTH shells calling it. Dialogs had
// the first and neither of the others — runtime.SetDialogsProvider existed, no
// backend/gomobile file exposed it, and no shell registered one — so every goleo:dialog*
// call on Android and iOS returned "no native provider registered". Nothing failed at
// build time, because a provider nobody registers is a valid program.
func TestDialogsProviderIsWiredEndToEnd(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/app\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := generateBackendEntrypoints(dir); err != nil {
		t.Fatalf("generating backend entry points: %v", err)
	}

	generated, err := os.ReadFile(filepath.Join(dir, "backend", "gomobile", "dialogs.go"))
	if err != nil {
		t.Fatalf("goleo does not generate backend/gomobile/dialogs.go, so gomobile emits no "+
			"SetDialogsProvider and no shell can register one: %v", err)
	}
	src := string(generated)

	// The tag must match runtime/dialogs_reexport.go's, or the file references a runtime
	// symbol that is not compiled in and the whole gomobile package fails to build.
	if !strings.Contains(src, "//go:build mobilebuild && goleo_dialog") {
		t.Errorf("dialogs.go must carry the same goleo_dialog tag as the runtime re-export:\n%s", src)
	}
	if !strings.Contains(src, "func SetDialogsProvider(") {
		t.Error("dialogs.go must expose SetDialogsProvider to the native shells")
	}

	// Each method must take a string and return a LONE string. Two separate gobind traps
	// meet here:
	//
	//  1. It cannot bind the runtime's option structs across packages, and it does not
	//     reject a method it cannot bind — it OMITS it from the generated proxy, so the
	//     failure arrives at runtime as an unrecognised selector on a device.
	//  2. A (string, error) result compiles on Android and breaks iOS. gobind emits a
	//     _Nonnull NSString return alongside error:(NSError**), Swift will not rewrite that
	//     into a throwing method, and no Swift signature conforms to the protocol. That
	//     shipped in 0.10.7 and had to be fixed in 0.10.8.
	//
	// Hence the envelope: failures come back as {"error":"..."} inside the reply.
	for _, method := range []string{
		"OpenFileJSON(optsJSON string) string",
		"SaveFileJSON(optsJSON string) string",
		"SelectFolderJSON(optsJSON string) string",
		"ShowMessageJSON(optsJSON string) string",
		"ShowPromptJSON(optsJSON string) string",
	} {
		if !strings.Contains(src, method) {
			t.Errorf("DialogsProvider is missing %q — every method must take and return a "+
				"JSON string, with no error result", method)
		}
	}
	if strings.Contains(src, "JSON(optsJSON string) (string, error)") {
		t.Error("a DialogsProvider method returns (string, error). gobind emits a _Nonnull " +
			"NSString return with error:(NSError**), which Swift will not import as a " +
			"throwing method, so the iOS shell cannot conform to the generated protocol. " +
			"Errors belong in the reply envelope instead. This is what broke 0.10.7.")
	}
}

// The iOS shell must not declare the dialog methods as throwing, for the same reason: the
// generated protocol methods are non-throwing, so `throws` does not satisfy them.
func TestIOSDialogMethodsAreNotThrowing(t *testing.T) {
	swift, err := mobileTemplates.ReadFile("templates/ios/App/AppDelegate.swift")
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{
		"openFileJSON", "saveFileJSON", "selectFolderJSON", "showMessageJSON", "showPromptJSON",
	} {
		if strings.Contains(string(swift), "func "+method+"(_ optsJSON: String?) throws -> String") {
			t.Errorf("GoleoDialogs.%s is declared `throws`, which cannot satisfy the "+
				"generated non-throwing protocol method — the 0.10.7 iOS build failure", method)
		}
		if !strings.Contains(string(swift), "func "+method+"(_ optsJSON: String?) -> String") {
			t.Errorf("GoleoDialogs.%s must be `func %s(_ optsJSON: String?) -> String`",
				method, method)
		}
	}
}

// Dialog calls must be serialised, and must refuse to run on the UI thread.
//
// Both matter because the bridge runs every invoke on its own goroutine
// (runtime/websocket.go), so two dialogs in flight at once is ordinary. The file picker in
// particular parks its result in state shared with the activity/app delegate, so without a
// lock one call consumes the other's result and the loser blocks forever — and these
// methods block with no timeout, so "forever" is literal.
func TestDialogCallsAreSerialisedAndOffTheUIThread(t *testing.T) {
	swift, err := mobileTemplates.ReadFile("templates/ios/App/AppDelegate.swift")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(swift), "serial.wait()") {
		t.Error("iOS dialog presentation must be serialised; two concurrent openFile calls " +
			"otherwise share one pickerDelegate slot and the first never completes")
	}
	if !strings.Contains(string(swift), "Thread.isMainThread") {
		t.Error("iOS dialogs must refuse the main thread rather than deadlock on it")
	}

	for _, variant := range []string{"android", "android-dev"} {
		java, err := mobileTemplates.ReadFile(
			"templates/" + variant + "/app/src/main/java/com/goleo/app/MainActivity.java")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(java), "withDialogLock(") {
			t.Errorf("%s dialog calls must go through withDialogLock; openFileJSON shares "+
				"documentPickLatch/documentPickResult with the launcher callback", variant)
		}
		if !strings.Contains(string(java), "Looper.myLooper() == Looper.getMainLooper()") {
			t.Errorf("%s dialogs must refuse the UI thread: runOnUiThread runs inline when "+
				"already there, so await() would freeze the thread drawing the dialog", variant)
		}
	}
}

// A provider the shells wire unconditionally must have its tag forced into every mobile
// build, or gobind emits no binding for an app that does not enable that feature and the
// shell fails to compile with "cannot find symbol gomobile.XProvider".
//
// This is invisible in the demo scaffold, which enables everything — it only breaks a
// minimal project, which is what `goleo new` produces by default.
func TestUnconditionallyWiredProvidersHaveTheirTagForced(t *testing.T) {
	forced := map[string]bool{}
	for _, tag := range nativeShellProviderTags {
		forced[tag] = true
	}
	// Provider symbol as the shells reference it -> the tag its Go binding lives behind.
	// Notifier is absent deliberately: it has no goleo_* tag and is always compiled in.
	for provider, tag := range map[string]string{
		"BatteryProvider":    "goleo_battery",
		"WakeLockProvider":   "goleo_wakelock",
		"SensorsProvider":    "goleo_sensors",
		"BackgroundProvider": "goleo_background",
		"NFCProvider":        "goleo_nfc",
		"BLEProvider":        "goleo_ble",
		"ClipboardProvider":  "goleo_clipboard",
		"ShareProvider":      "goleo_share",
		"DialogsProvider":    "goleo_dialog",
	} {
		if !forced[tag] {
			t.Errorf("the shells wire %s unconditionally but %s is not in "+
				"nativeShellProviderTags, so any app that does not enable the feature "+
				"fails to compile its native shell", provider, tag)
		}
	}
}

// Every provider the gomobile bridge exposes must be registered by BOTH shells. A
// provider registered on only one platform is the harder bug to find: the feature works
// on the device you happen to test.
func TestBothShellsRegisterEveryProvider(t *testing.T) {
	swift, err := mobileTemplates.ReadFile("templates/ios/App/AppDelegate.swift")
	if err != nil {
		t.Fatal(err)
	}
	// BOTH Android shells: android-dev is a separate fixed source file, so a provider added
	// to only one of them works in `goleo build android` and not in `goleo emulate android`,
	// or the reverse. Reading them by explicit path rather than globbing, so a third shell
	// added later fails this test rather than being silently skipped.
	shells := map[string][]byte{}
	for _, variant := range []string{"android", "android-dev"} {
		java, err := mobileTemplates.ReadFile(
			"templates/" + variant + "/app/src/main/java/com/goleo/app/MainActivity.java")
		if err != nil {
			t.Fatal(err)
		}
		shells[variant] = java
	}

	// Go's SetXProvider becomes GomobileSetXProvider in Swift and Gomobile.setXProvider in
	// Java — see the naming note in AppDelegate.swift.
	for _, provider := range []string{
		"Notifier", "BatteryProvider", "WakeLockProvider", "SensorsProvider",
		"BackgroundProvider", "ClipboardProvider", "ShareProvider", "DialogsProvider",
	} {
		if !strings.Contains(string(swift), "GomobileSet"+provider+"(") {
			t.Errorf("the iOS shell never calls GomobileSet%s — the feature compiles in and "+
				"then fails at runtime with 'no native provider registered'", provider)
		}
		for variant, java := range shells {
			if !strings.Contains(string(java), "Gomobile.set"+provider+"(") {
				t.Errorf("the %s shell never calls Gomobile.set%s — the feature compiles in "+
					"and then fails at runtime with 'no native provider registered'",
					variant, provider)
			}
		}
	}
}
