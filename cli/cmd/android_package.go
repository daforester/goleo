package cmd

import (
	"fmt"
	"strings"
)

// javaKeywords cannot appear as a package segment.
//
// mobile.android.package_name is interpolated into `package {{.PackageName}};` in the
// generated MainActivity.java and GoleoSyncWorker.java, so it has to be a valid Java
// package name, not merely a valid Android applicationId. `com.example.new` is an
// accepted applicationId as far as the manifest is concerned and a hard javac error.
var javaKeywords = map[string]bool{
	"abstract": true, "assert": true, "boolean": true, "break": true, "byte": true,
	"case": true, "catch": true, "char": true, "class": true, "const": true,
	"continue": true, "default": true, "do": true, "double": true, "else": true,
	"enum": true, "extends": true, "final": true, "finally": true, "float": true,
	"for": true, "goto": true, "if": true, "implements": true, "import": true,
	"instanceof": true, "int": true, "interface": true, "long": true, "native": true,
	"new": true, "package": true, "private": true, "protected": true, "public": true,
	"return": true, "short": true, "static": true, "strictfp": true, "super": true,
	"switch": true, "synchronized": true, "this": true, "throw": true, "throws": true,
	"transient": true, "try": true, "void": true, "volatile": true, "while": true,
	// Not reserved words but reserved literals, equally invalid as identifiers.
	"true": true, "false": true, "null": true,
	// Contextual/reserved in modern Java; safe to refuse rather than explain later.
	"_": true, "record": true, "sealed": true, "permits": true, "var": true, "yield": true,
}

// validateAndroidPackageName checks mobile.android.package_name before any build work.
//
// Nothing validated it, so a bad value failed inside javac or the Gradle manifest merger
// with an error that names a generated file under .goleo/android rather than the line of
// goleo.json that caused it. It is also worth catching early because the package name is
// PERMANENT once an artifact reaches Play: it identifies the listing, cannot be changed,
// and cannot be reused even after unpublishing.
func validateAndroidPackageName(pkg string) error {
	name := strings.TrimSpace(pkg)
	if name == "" {
		return fmt.Errorf("mobile.android.package_name is empty")
	}
	if name != pkg {
		return fmt.Errorf("mobile.android.package_name %q has leading or trailing whitespace", pkg)
	}

	segments := strings.Split(name, ".")
	if len(segments) < 2 {
		return fmt.Errorf("mobile.android.package_name %q needs at least two dot-separated "+
			"segments (e.g. com.example.myapp) — Android requires it and Play rejects a "+
			"single-segment id", name)
	}

	for _, seg := range segments {
		if seg == "" {
			return fmt.Errorf("mobile.android.package_name %q has an empty segment "+
				"(a leading, trailing or doubled dot)", name)
		}
		if c := seg[0]; !isASCIILetter(c) {
			return fmt.Errorf("mobile.android.package_name %q: segment %q must start with a "+
				"letter — it becomes a Java package, so a digit or underscore there is a "+
				"compile error", name, seg)
		}
		for i := 0; i < len(seg); i++ {
			c := seg[i]
			if !isASCIILetter(c) && !isASCIIDigit(c) && c != '_' {
				return fmt.Errorf("mobile.android.package_name %q: segment %q contains %q — "+
					"only letters, digits and underscore are allowed (hyphens are a common "+
					"mistake; they are legal in a domain but not in a package name)",
					name, seg, string(c))
			}
		}
		if javaKeywords[strings.ToLower(seg)] {
			return fmt.Errorf("mobile.android.package_name %q: segment %q is a Java reserved "+
				"word, so the generated MainActivity.java would not compile", name, seg)
		}
	}
	return nil
}

func isASCIILetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isASCIIDigit(c byte) bool { return c >= '0' && c <= '9' }
