package store

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// configDirEnv returns the environment variable os.UserConfigDir actually reads on
// this platform. Getting this wrong makes a redirect a silent no-op — on Windows
// os.UserConfigDir reads %AppData%, while %LocalAppData% is os.UserCacheDir, and a
// test that sets the wrong one quietly measures the developer's real profile (and
// writes into it).
func configDirEnv() string {
	switch runtime.GOOS {
	case "windows":
		return "APPDATA"
	case "darwin":
		return "HOME" // -> $HOME/Library/Application Support
	default:
		return "XDG_CONFIG_HOME"
	}
}

// TestStoreUpgradeAcrossProcesses exercises the rename the way a user actually
// meets it: an app built BEFORE the change writes to the shared
// <cfg>/goleo-app/store.json, then the upgraded build — which calls SetAppName —
// must still find those keys.
//
// This has to span processes. Default() is guarded by a sync.Once, so within one
// process the store path is fixed the first time anything touches it and the
// upgrade can never be replayed. The child re-executes this same test binary with
// GOLEO_STORE_PHASE set.
func TestStoreUpgradeAcrossProcesses(t *testing.T) {
	switch os.Getenv("GOLEO_STORE_PHASE") {
	case "v1":
		// The app as it shipped before the change: never calls SetAppName.
		s, err := Default()
		if err != nil {
			t.Fatalf("v1 Default: %v", err)
		}
		if err := s.Set("token", json.RawMessage(`"secret-abc"`)); err != nil {
			t.Fatalf("v1 Set: %v", err)
		}
		if err := s.Set("count", json.RawMessage(`42`)); err != nil {
			t.Fatalf("v1 Set: %v", err)
		}
		return

	case "v2":
		// The upgraded app, naming itself as App.New does.
		SetAppName("upgrade-demo")
		s, err := Default()
		if err != nil {
			t.Fatalf("v2 Default: %v", err)
		}
		tok, okTok := s.Get("token")
		cnt, okCnt := s.Get("count")
		if !okTok || !okCnt {
			t.Fatalf("DATA LOSS after upgrade: token=%v count=%v keys=%v", okTok, okCnt, s.Keys())
		}
		if string(tok) != `"secret-abc"` || string(cnt) != `42` {
			t.Fatalf("values changed in migration: token=%s count=%s", tok, cnt)
		}
		return
	}

	// Parent: drive the two phases against an isolated config dir.
	cfg := t.TempDir()
	run := func(phase string) {
		t.Helper()
		cmd := exec.Command(os.Args[0], "-test.run=TestStoreUpgradeAcrossProcesses", "-test.v")
		cmd.Env = append(os.Environ(),
			"GOLEO_STORE_PHASE="+phase,
			configDirEnv()+"="+cfg,
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s phase failed: %v\n%s", phase, err, out)
		}
	}

	run("v1")

	// Guard against the redirect silently not applying — otherwise this test would
	// write into the developer's real profile and pass without proving anything.
	legacy := filepath.Join(cfg, defaultAppName, storeFile)
	if _, err := os.Stat(legacy); err != nil {
		t.Fatalf("v1 did not write to the redirected config dir (%s): %v — is %s the right variable on %s?",
			legacy, err, configDirEnv(), runtime.GOOS)
	}

	run("v2")

	migrated := filepath.Join(cfg, "upgrade-demo", storeFile)
	data, err := os.ReadFile(migrated)
	if err != nil {
		t.Fatalf("upgraded app has no store of its own at %s: %v", migrated, err)
	}
	if !strings.Contains(string(data), "secret-abc") {
		t.Errorf("migrated store is missing the original data: %s", data)
	}
	// Copy, not move — other apps may still be reading the legacy file.
	if _, err := os.Stat(legacy); err != nil {
		t.Errorf("legacy store should survive the migration, got %v", err)
	}
}
