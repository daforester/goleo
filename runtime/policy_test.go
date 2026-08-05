package runtime

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestPolicyAllowsMethod(t *testing.T) {
	p := &Policy{Allow: []string{"goleo:store*", "goleo:fsReadTextFile"}}

	cases := map[string]bool{
		"goleo:getOS":           true,  // always-allowed core
		"goleo:capabilities":    true,  // always-allowed core
		"goleo:storeGet":        true,  // prefix match
		"goleo:storeSet":        true,  // prefix match
		"goleo:fsReadTextFile":  true,  // exact match
		"goleo:fsWriteTextFile": false, // not allowed
		"goleo:shareShare":      false, // not allowed
		"goleo:getEnv":          false, // sensitive core, not auto-allowed
	}
	for method, want := range cases {
		if got := p.allowsMethod(method); got != want {
			t.Errorf("allowsMethod(%q) = %v, want %v", method, got, want)
		}
	}
}

func TestPolicyFSScope(t *testing.T) {
	root := filepath.Join("home", "user", "app")
	p := &Policy{FSRoots: []string{root}}

	if !p.AllowsFSPath(filepath.Join(root, "data.json")) {
		t.Error("path within root should be allowed")
	}
	if !p.AllowsFSPath(root) {
		t.Error("the root itself should be allowed")
	}
	if p.AllowsFSPath(filepath.Join("home", "user", "other", "x")) {
		t.Error("path outside root should be denied")
	}
	// Traversal cannot escape the root after cleaning.
	if p.AllowsFSPath(filepath.Join(root, "..", "..", "etc", "passwd")) {
		t.Error("traversal out of root should be denied")
	}
	// Empty roots = unconstrained.
	if !(&Policy{}).AllowsFSPath("/anywhere") {
		t.Error("empty FSRoots should be unconstrained")
	}
}

func TestPolicyScopeLists(t *testing.T) {
	p := &Policy{HTTPHosts: []string{"api.example.com"}, ShellPrograms: []string{"git"}}
	if !p.AllowsHTTPHost("api.example.com") || p.AllowsHTTPHost("evil.com") {
		t.Error("http host scope wrong")
	}
	if !p.AllowsShellProgram("git") || p.AllowsShellProgram("rm") {
		t.Error("shell program scope wrong")
	}
	if !(&Policy{}).AllowsHTTPHost("anything") {
		t.Error("empty host list should be unconstrained")
	}
}

// TestBridgeEnforcesPolicy verifies enforcement happens centrally in dispatch.
func TestBridgeEnforcesPolicy(t *testing.T) {
	b := NewBridge()
	called := false
	b.Handle("goleo:secret", func(ctx context.Context, args json.RawMessage) (any, error) {
		called = true
		return "ok", nil
	})
	b.Handle("goleo:allowed", func(ctx context.Context, args json.RawMessage) (any, error) {
		return "ok", nil
	})

	// No policy → permissive.
	if resp := b.HandleRequest(InvokeRequest{ID: "1", Method: "goleo:secret"}); resp.Error != "" {
		t.Fatalf("no policy should permit: %s", resp.Error)
	}

	// With a policy, the un-allowed method is denied before the handler runs.
	b.SetPolicy(&Policy{Allow: []string{"goleo:allowed"}})
	called = false
	resp := b.HandleRequest(InvokeRequest{ID: "2", Method: "goleo:secret"})
	if resp.Error == "" {
		t.Error("policy should deny goleo:secret")
	}
	if called {
		t.Error("denied handler must NOT run")
	}
	if resp := b.HandleRequest(InvokeRequest{ID: "3", Method: "goleo:allowed"}); resp.Error != "" {
		t.Errorf("allowed method should pass: %s", resp.Error)
	}
	// Core stays allowed even under a restrictive policy.
	b.Handle("goleo:getOS", func(ctx context.Context, args json.RawMessage) (any, error) { return nil, nil })
	if resp := b.HandleRequest(InvokeRequest{ID: "4", Method: "goleo:getOS"}); resp.Error != "" {
		t.Errorf("core getOS should always pass: %s", resp.Error)
	}
}
