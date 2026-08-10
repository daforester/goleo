package cmd

import (
	"os"
	"strings"
	"testing"
)

// Every gomobile invocation must pass a RESOLVED Android API level.
//
// `goleo emulate android` used to pass the raw androidAPI global. That global is
// `goleo build`'s --android-api flag — and emulate does not even declare that flag — so it
// was ALWAYS 0 there. gomobile validates the level against the NDK's meta/platforms.json
// (present since NDK r23, i.e. every current NDK) and refused with:
//
//	ANDROID_NDK_HOME specifies .../ndk/28.2.13676358, which is unusable:
//	unsupported API version 0 (not in 21..35)
//
// That message names the NDK, so it reads as a broken SDK install rather than as goleo
// passing a nonsense value — which is why it went unrecognised. `goleo build android`
// resolved the level correctly the whole time: two code paths building the same argument
// list, only one of them right.
func TestEveryGomobileCallSiteResolvesTheAndroidAPI(t *testing.T) {
	for _, file := range []string{"build.go", "emulate.go"} {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		sites := 0
		for i, line := range strings.Split(string(src), "\n") {
			if !strings.Contains(line, `"-androidapi"`) {
				continue
			}
			sites++
			if !strings.Contains(line, "minAPI") {
				t.Errorf("%s:%d passes -androidapi without resolving it:\n\t%s\n"+
					"Use resolveAndroidMinAPI(androidAPI, cfg.MinSDK). The raw flag is 0 "+
					"unless the user set it, and gomobile rejects 0 with a message that "+
					"blames the NDK.", file, i+1, strings.TrimSpace(line))
			}
		}
		if sites == 0 {
			t.Errorf("%s no longer passes -androidapi to gomobile — if that moved, move "+
				"this guard with it rather than deleting it", file)
		}
	}
}

// The resolver's own floor. With neither a flag nor mobile.android.min_sdk, the default has
// to sit inside the range gomobile will accept, or every default Android build fails before
// compiling anything.
func TestDefaultAndroidAPIIsAcceptableToGomobile(t *testing.T) {
	got, err := resolveAndroidMinAPI(0, 0)
	if err != nil {
		t.Fatalf("resolving the default android API: %v", err)
	}
	// 21 is the low end of what NDK 23..28 report in meta/platforms.json.
	if got < 21 {
		t.Errorf("the default Android API resolves to %d; gomobile's NDK check rejects "+
			"anything below 21, so every default build would fail", got)
	}
}
