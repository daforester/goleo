package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// localReplaceTarget returns the directory a `replace <module> => <dir>` points at, or "" if
// there is no such replace or the replacement is another MODULE rather than a directory.
//
// Go distinguishes the two by the presence of a version: a directory replacement must not
// carry one (`replace x => ../y`), while a module replacement must (`replace x => other/y
// v1.2.3`). Testing for the version is more reliable than pattern-matching the path, which
// would have to cope with `./`, `../`, `/abs` and Windows drive letters.
func localReplaceTarget(modContent, module string) string {
	inBlock := false
	for _, line := range strings.Split(modContent, "\n") {
		trimmed := strings.TrimSpace(strings.TrimSuffix(line, "\r"))

		// Strip a trailing line comment first, and skip whole-line comments — a
		// commented-out example must not register as real.
		if i := strings.Index(trimmed, "//"); i >= 0 {
			trimmed = strings.TrimSpace(trimmed[:i])
		}
		if trimmed == "" {
			continue
		}

		// go.mod allows a block form, which `go mod edit` does not write but a
		// hand-edited file may well use:
		//
		//   replace (
		//       github.com/x/y => ../y
		//   )
		switch {
		case trimmed == "replace (":
			inBlock = true
			continue
		case inBlock && trimmed == ")":
			inBlock = false
			continue
		case strings.HasPrefix(trimmed, "replace "):
			trimmed = strings.TrimPrefix(trimmed, "replace ")
		case !inBlock:
			continue
		}

		// Split on the arrow rather than on "<module> =>", so any amount of
		// whitespace works. Matching a fixed single space is how the first version
		// of this missed a hand-formatted go.mod — and a warning that silently does
		// not fire is worse than no warning, since silence reads as "all clear".
		lhs, rhs, found := strings.Cut(trimmed, "=>")
		if !found {
			continue
		}
		lf := strings.Fields(lhs)
		if len(lf) == 0 || lf[0] != module {
			continue
		}
		switch fields := strings.Fields(rhs); len(fields) {
		case 1:
			return fields[0] // no version => a directory
		default:
			return "" // module + version => a deliberate fork pin, not a stray local path
		}
	}
	return ""
}

// warnDetachedLocalReplace flags a project whose go.mod points goleo at a local directory
// while GOLEO_ROOT is NOT set.
//
// GOLEO_ROOT injects that replace and nothing removes it again: snapshotModFiles guards only
// the mobile and emulate paths, so a single `goleo build` or `goleo dev` with the variable set
// repoints the project permanently. The effect then OUTLIVES the variable — every later build
// silently compiles the checkout and the `require` line becomes decorative.
//
// That is not hypothetical. A real project was found requiring v0.9.3 while actually building a
// working tree several releases ahead, which also meant a release verification had not tested
// the released module at all. Bumping the require would have looked like an upgrade and changed
// nothing.
//
// Warn rather than fix: the replace is legitimate and wanted while developing goleo. What is
// indefensible is it being invisible.
func warnDetachedLocalReplace(projectDir string) {
	if os.Getenv("GOLEO_ROOT") != "" {
		return // intentional on this run, and already announced by the caller
	}
	data, err := os.ReadFile(filepath.Join(projectDir, "go.mod"))
	if err != nil {
		return
	}
	target := localReplaceTarget(string(data), goleoModule)
	if target == "" {
		return
	}
	fmt.Printf("\n  NOTE: go.mod replaces %s with a local directory:\n", goleoModule)
	fmt.Printf("          %s\n", target)
	fmt.Println("  This build compiles THAT directory, so the require version in go.mod is not")
	fmt.Println("  what you are shipping — and GOLEO_ROOT is not set on this run. Expected while")
	fmt.Println("  developing goleo itself; otherwise drop it to build a released version:")
	fmt.Printf("          go mod edit -dropreplace %s\n", goleoModule)
	fmt.Println("          go mod tidy && go mod vendor")
	fmt.Println()
}
