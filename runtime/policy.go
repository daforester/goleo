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
// SCOPE LISTS ARE NOT YET ENFORCED. Allow (method-level) is. FSRoots, HTTPHosts
// and ShellPrograms are stored and have Allows* helpers, but no plugin calls
// them, so populating them restricts nothing — do not treat FSRoots as
// filesystem confinement. Until they are wired in, keep a sensitive plugin out
// of Allow (or don't register it) rather than relying on a scope list.
type Policy struct {
	// Allow lists permitted invoke methods. "goleo:store*" allows the whole
	// store plugin; "goleo:fsReadTextFile" allows exactly one command.
	// Enforced by allowsMethod via Bridge.HandleRequest.
	Allow []string
	// FSRoots is intended to limit filesystem access to these path prefixes.
	// NOT YET ENFORCED — see the type comment.
	FSRoots []string
	// HTTPHosts is intended to limit the http plugin to these hosts.
	// NOT YET ENFORCED — see the type comment.
	HTTPHosts []string
	// ShellPrograms is intended to limit the shell plugin to these program names.
	// NOT YET ENFORCED — see the type comment.
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
