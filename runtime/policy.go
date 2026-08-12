package runtime

import (
	"path/filepath"
	"strings"
)

// Policy is a runtime capability ACL. When set on a Bridge (SetPolicy), every
// invoke is checked centrally before its handler runs: the method must be in
// Allow (exact match, or a "prefix*" wildcard) or an always-safe core command,
// otherwise it is denied.
//
// No policy set = no enforcement (legacy-permissive). Setting a policy opts into
// deny-by-default, matching Tauri's capability model.
//
// FSRoots IS enforced (since the fs-scope change): SetPolicy registers each root
// with the Bridge's filesystem scope, and every fs handler checks it. HTTPHosts
// and ShellPrograms are reserved — goleo has no http or shell plugin yet, so
// there is nothing for them to gate; they are accepted so a policy written today
// keeps working when those plugins land.
type Policy struct {
	// Allow lists permitted invoke methods. "goleo:store*" allows the whole
	// store plugin; "goleo:fsReadTextFile" allows exactly one command.
	// Enforced by allowsMethod via Bridge.HandleRequest.
	Allow []string
	// FSRoots widens the filesystem plugin's scope to these directories, on top
	// of the app's own data directory and any path the user picks in a native
	// dialog. Enforced via Bridge.checkFSPath — see fs_scope.go.
	FSRoots []string
	// HTTPHosts is intended to limit an http plugin to these hosts.
	// RESERVED — no http plugin exists yet, so this gates nothing today.
	HTTPHosts []string
	// ShellPrograms is intended to limit a shell plugin to these program names.
	// RESERVED — no shell plugin exists yet, so this gates nothing today.
	ShellPrograms []string
}

// alwaysAllowed are safe, info-only core commands permitted regardless of Allow,
// so a restrictive policy can't accidentally lock out basic bridge use.
var alwaysAllowed = map[string]bool{
	"goleo:getOS":        true,
	"goleo:getPlatform":  true,
	"goleo:getArch":      true,
	"goleo:capabilities": true,
}

func (p *Policy) allowsMethod(method string) bool {
	if alwaysAllowed[method] {
		return true
	}
	for _, a := range p.Allow {
		if a == method {
			return true
		}
		if strings.HasSuffix(a, "*") && strings.HasPrefix(method, strings.TrimSuffix(a, "*")) {
			return true
		}
	}
	return false
}

// AllowsFSPath reports whether path is within an allowed root. Empty FSRoots =
// unconstrained. Uses cleaned paths so "../" traversal cannot escape a root.
//
// NOTE: this is a raw helper for hosts doing their own checks; it is NOT the
// enforcement path. Enforcement is Bridge.checkFSPath (fs_scope.go), which treats
// FSRoots as additive to the app data directory and dialog grants, resolves
// symlinks, and applies a deny-list — none of which this helper does. In
// particular its "empty means unconstrained" rule is the opposite of the default
// the fs plugin needs, so do not use it to decide access.
func (p *Policy) AllowsFSPath(path string) bool {
	if len(p.FSRoots) == 0 {
		return true
	}
	clean := filepath.Clean(path)
	for _, root := range p.FSRoots {
		r := filepath.Clean(root)
		if clean == r || strings.HasPrefix(clean, r+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// AllowsHTTPHost reports whether host is permitted. Empty HTTPHosts = unconstrained.
func (p *Policy) AllowsHTTPHost(host string) bool { return listAllows(p.HTTPHosts, host) }

// AllowsShellProgram reports whether program is permitted. Empty = unconstrained.
func (p *Policy) AllowsShellProgram(program string) bool { return listAllows(p.ShellPrograms, program) }

func listAllows(list []string, v string) bool {
	if len(list) == 0 {
		return true
	}
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
