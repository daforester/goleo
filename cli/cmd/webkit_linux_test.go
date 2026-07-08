//go:build linux

package cmd

import (
	"reflect"
	"testing"
)

func TestParseWebkit4x(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "only 4.1 (typical current distro)",
			in: "gtk+-3.0                       GTK+ - GTK+ Graphical UI Library\n" +
				"webkit2gtk-4.1                 WebKitGTK - Web content engine for GTK\n" +
				"webkit2gtk-web-extension-4.1   WebKitGTK web process extensions\n",
			want: []string{"webkit2gtk-4.1"},
		},
		{
			name: "4.0 and 4.1 both present, unsorted input",
			in: "webkit2gtk-4.1   WebKitGTK\n" +
				"webkit2gtk-4.0   WebKitGTK\n",
			want: []string{"webkit2gtk-4.0", "webkit2gtk-4.1"},
		},
		{
			name: "future 4.2 alongside 4.1",
			in: "webkit2gtk-4.1   WebKitGTK\n" +
				"webkit2gtk-4.2   WebKitGTK\n",
			want: []string{"webkit2gtk-4.1", "webkit2gtk-4.2"},
		},
		{
			name: "double-digit minor sorts numerically, not lexically",
			in: "webkit2gtk-4.10   WebKitGTK\n" +
				"webkit2gtk-4.2    WebKitGTK\n" +
				"webkit2gtk-4.1    WebKitGTK\n",
			want: []string{"webkit2gtk-4.1", "webkit2gtk-4.2", "webkit2gtk-4.10"},
		},
		{
			name: "excludes web-extension and unrelated webkit modules",
			in: "webkit2gtk-web-extension-4.1   web process extensions\n" +
				"javascriptcoregtk-4.1          JavaScriptCore\n" +
				"webkitgtk-6.0                  WebKitGTK GTK4 API\n" +
				"webkit2gtk-4.1                 WebKitGTK\n",
			want: []string{"webkit2gtk-4.1"},
		},
		{
			name: "no webkit at all",
			in:   "gtk+-3.0   GTK+\nglib-2.0   GLib\n",
			want: []string{},
		},
		{
			name: "empty input",
			in:   "",
			want: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseWebkit4x(tt.in)
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseWebkit4x() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestChooseWebkit(t *testing.T) {
	tests := []struct {
		name         string
		mods         []string
		wantTarget   string
		wantNeedShim bool
		wantOK       bool
	}{
		{
			name:   "nothing installed",
			mods:   nil,
			wantOK: false,
		},
		{
			name:         "4.0 present -> native, no shim",
			mods:         []string{"webkit2gtk-4.0", "webkit2gtk-4.1"},
			wantTarget:   "webkit2gtk-4.0",
			wantNeedShim: false,
			wantOK:       true,
		},
		{
			name:         "only 4.1 -> shim toward 4.1",
			mods:         []string{"webkit2gtk-4.1"},
			wantTarget:   "webkit2gtk-4.1",
			wantNeedShim: true,
			wantOK:       true,
		},
		{
			name:         "future 4.1 + 4.2, no 4.0 -> shim toward newest (4.2)",
			mods:         []string{"webkit2gtk-4.1", "webkit2gtk-4.2"},
			wantTarget:   "webkit2gtk-4.2",
			wantNeedShim: true,
			wantOK:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, needShim, ok := chooseWebkit(tt.mods)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if target != tt.wantTarget {
				t.Errorf("target = %q, want %q", target, tt.wantTarget)
			}
			if needShim != tt.wantNeedShim {
				t.Errorf("needShim = %v, want %v", needShim, tt.wantNeedShim)
			}
		})
	}
}
