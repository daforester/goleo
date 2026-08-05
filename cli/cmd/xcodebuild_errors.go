package cmd

import (
	"bytes"
	"fmt"
	"regexp"
)

// xcodeFutureFormatRe matches xcodebuild's refusal to open a project written in a newer
// format than the installed Xcode understands. The parenthesised number is the pbxproj
// objectVersion.
var xcodeFutureFormatRe = regexp.MustCompile(`future Xcode project file format \((\d+)\)`)

// objectVersionXcode maps a pbxproj objectVersion to the Xcode release that introduced it,
// so the error can name the version the user would need. From XcodeGen's ProjectFormat.
var objectVersionXcode = map[string]string{
	"56": "14.0",
	"60": "15.0",
	"63": "15.3",
	"77": "16.0",
	"90": "16.3",
}

// explainXcodebuildFailure turns an xcodebuild failure into something actionable when the
// output identifies a cause goleo knows about. Otherwise it wraps the error unchanged —
// xcodebuild's own output has already been streamed to the terminal.
//
// The case that motivated this: XcodeGen defaults to writing objectVersion 77 (Xcode
// 16.0's format), so on any older Xcode every iOS build failed with
//
//	The project 'GoleoApp' cannot be opened because it is in a future Xcode project
//	file format (77).                             xcodebuild failed: exit status 74
//
// which names neither xcodegen, nor the generated file, nor the Xcode version needed. The
// template now pins projectFormat, but a user can still hit this by overriding it, or with
// an XcodeGen too old to know the key (an unrecognised value falls back to 77).
func explainXcodebuildFailure(output string, err error) error {
	if m := xcodeFutureFormatRe.FindStringSubmatch(output); m != nil {
		objVersion := m[1]
		needs := ""
		if v, ok := objectVersionXcode[objVersion]; ok {
			needs = fmt.Sprintf(" (Xcode %s or newer)", v)
		}
		return fmt.Errorf("the generated Xcode project is in a format your Xcode cannot open: "+
			"objectVersion %s%s.\n"+
			"  xcodegen wrote it, not goleo — its default project format tracks the newest\n"+
			"  Xcode rather than yours. Either upgrade Xcode, or lower `options.projectFormat`\n"+
			"  in .goleo/ios/xcodegen.yml (xcode14_0 is the most compatible) and rebuild.\n"+
			"  xcodebuild: %w", objVersion, needs, err)
	}
	return fmt.Errorf("xcodebuild failed: %w", err)
}

// teeBuffer accumulates up to a cap of what is written through it, so a subprocess's
// output can be inspected after the fact while still streaming to the terminal.
//
// Capped because xcodebuild is extremely verbose and there is no reason to hold a whole
// build log in memory to look for one line.
type teeBuffer struct {
	buf bytes.Buffer
	max int
}

func (t *teeBuffer) Write(p []byte) (int, error) {
	if t.buf.Len() < t.max {
		room := t.max - t.buf.Len()
		if room > len(p) {
			room = len(p)
		}
		t.buf.Write(p[:room])
	}
	return len(p), nil
}

func (t *teeBuffer) String() string { return t.buf.String() }
