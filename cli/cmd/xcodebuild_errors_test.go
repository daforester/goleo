package cmd

import (
	"errors"
	"strings"
	"testing"
)

// The real output from the macos-14 runner. Kept verbatim so the matcher is tested against
// what xcodebuild actually prints, not a paraphrase of it.
const realFutureFormatOutput = `2026-08-04 21:31:13.015 xcodebuild[10982:36032] Writing error result bundle to /var/folders/g3/T/ResultBundle.xcresult
xcodebuild: error: Unable to read project 'GoleoApp.xcodeproj' from folder '/private/tmp/ios/iosapp/.goleo/ios'.
	Reason: The project ` + "‘GoleoApp’" + ` cannot be opened because it is in a future Xcode project file format (77). Adjust the project format using a compatible version of Xcode to allow it to be opened by this version of Xcode.
`

func TestFutureProjectFormatFailureIsExplained(t *testing.T) {
	err := explainXcodebuildFailure(realFutureFormatOutput, errors.New("exit status 74"))
	if err == nil {
		t.Fatal("a failed build must still be an error")
	}
	msg := err.Error()

	// The three things "xcodebuild failed: exit status 74" did not tell you: what is
	// wrong, who wrote the file, and what to do.
	for _, want := range []string{
		"objectVersion 77",
		"Xcode 16.0 or newer", // the version this format needs
		"xcodegen wrote it",   // not goleo, and not something the user typed
		"projectFormat",       // the knob
		".goleo/ios/xcodegen.yml",
		"xcode14_0",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("the explanation should mention %q:\n%s", want, msg)
		}
	}
	// And the underlying error must still be wrapped, so exit codes are not lost.
	if !strings.Contains(msg, "exit status 74") {
		t.Errorf("the original xcodebuild error should be preserved:\n%s", msg)
	}
}

func TestFutureProjectFormatMapsEveryFormatXcodegenCanWrite(t *testing.T) {
	// From XcodeGen's ProjectFormat: xcode14_0=56, xcode15_0=60, xcode15_3=63,
	// xcode16_0=77, xcode16_3=90. Any of these can appear depending on what the user's
	// xcodegen defaults to or what they set.
	for objVersion, xcode := range map[string]string{
		"56": "14.0", "60": "15.0", "63": "15.3", "77": "16.0", "90": "16.3",
	} {
		out := "Reason: The project cannot be opened because it is in a future Xcode project file format (" + objVersion + ")."
		err := explainXcodebuildFailure(out, errors.New("exit status 74"))
		if !strings.Contains(err.Error(), "Xcode "+xcode+" or newer") {
			t.Errorf("objectVersion %s should map to Xcode %s:\n%v", objVersion, xcode, err)
		}
	}

	// An objectVersion goleo has not seen must still produce the actionable advice, just
	// without naming a version — guessing one would be worse than omitting it.
	err := explainXcodebuildFailure(
		"future Xcode project file format (120).", errors.New("exit status 74"))
	if !strings.Contains(err.Error(), "objectVersion 120") {
		t.Errorf("an unknown objectVersion should still be named:\n%v", err)
	}
	if strings.Contains(err.Error(), "or newer") {
		t.Errorf("an unknown objectVersion must not be mapped to an invented Xcode version:\n%v", err)
	}
}

// Any other failure must pass through unchanged: xcodebuild has already printed its own
// diagnostics, and inventing an explanation for an unrelated failure is worse than none.
func TestOtherXcodebuildFailuresArePassedThrough(t *testing.T) {
	for _, out := range []string{
		"error: No signing certificate \"iOS Development\" found",
		"error: Signing for \"App\" requires a development team.",
		"** BUILD FAILED **",
		"",
	} {
		err := explainXcodebuildFailure(out, errors.New("exit status 65"))
		if !strings.Contains(err.Error(), "xcodebuild failed") {
			t.Errorf("unrelated failure %q should pass through:\n%v", out, err)
		}
		if strings.Contains(err.Error(), "projectFormat") {
			t.Errorf("unrelated failure %q was given the project-format explanation:\n%v", out, err)
		}
	}
}

// teeBuffer must not hold a whole xcodebuild log, and must never short-change the writer
// it stands in for — reporting fewer bytes written than it received would make
// io.MultiWriter fail the build with a short-write error.
func TestTeeBufferCapsWithoutShortWriting(t *testing.T) {
	tb := &teeBuffer{max: 10}
	chunk := []byte("0123456789abcdef")
	n, err := tb.Write(chunk)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(chunk) {
		t.Errorf("Write reported %d of %d bytes; a short write makes io.MultiWriter fail "+
			"the whole build", n, len(chunk))
	}
	if got := tb.String(); got != "0123456789" {
		t.Errorf("buffered %q, want the first 10 bytes only", got)
	}

	// Further writes are dropped, still without short-writing.
	n, err = tb.Write([]byte("more"))
	if err != nil || n != 4 {
		t.Errorf("Write after the cap returned (%d, %v), want (4, nil)", n, err)
	}
	if len(tb.String()) != 10 {
		t.Errorf("buffer grew past its cap to %d bytes", len(tb.String()))
	}
}

// The captured output is what the matcher sees, so the marker must survive being split
// across writes — xcodebuild's output arrives in arbitrary chunks.
func TestTeeBufferKeepsEnoughToMatchAcrossChunks(t *testing.T) {
	tb := &teeBuffer{max: 64 << 10}
	for _, part := range []string{
		"xcodebuild: error: Unable to read project.\n\tReason: it is in a ",
		"future Xcode project ",
		"file format (77). Adjust the project format.\n",
	} {
		if _, err := tb.Write([]byte(part)); err != nil {
			t.Fatal(err)
		}
	}
	err := explainXcodebuildFailure(tb.String(), errors.New("exit status 74"))
	if !strings.Contains(err.Error(), "objectVersion 77") {
		t.Errorf("a marker split across writes was not matched:\n%v", err)
	}
}
