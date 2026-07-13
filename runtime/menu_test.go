package runtime

import (
	"errors"
	"testing"
)

func TestStandardMenu(t *testing.T) {
	m := StandardMenu("Foo")
	if len(m) < 2 {
		t.Fatalf("StandardMenu: want >=2 top menus, got %d", len(m))
	}
	if m[0].Label != "Foo" {
		t.Errorf("first menu label = %q, want app name Foo", m[0].Label)
	}
	// App menu should carry a Quit role; Edit menu the clipboard roles.
	if !hasRole(m[0].Submenu, RoleQuit) {
		t.Error("app menu missing Quit role")
	}
	edit := m[1]
	for _, r := range []MenuRole{RoleCut, RoleCopy, RolePaste, RoleSelectAll} {
		if !hasRole(edit.Submenu, r) {
			t.Errorf("edit menu missing role %q", r)
		}
	}
}

func hasRole(items []MenuItem, role MenuRole) bool {
	for _, it := range items {
		if it.Role == role {
			return true
		}
	}
	return false
}

// TestSetMenuUnsupported: on platforms without a native menu bar (e.g. the
// Windows/Linux test host), SetMenu must report errors.ErrUnsupported rather
// than pretending to install a menu.
func TestSetMenuUnsupported(t *testing.T) {
	if MenuSupported() {
		t.Skip("native menu supported on this platform; nothing to assert here")
	}
	err := New(Config{}).SetMenu(StandardMenu("x"))
	if !errors.Is(err, errors.ErrUnsupported) {
		t.Fatalf("SetMenu on unsupported platform = %v, want ErrUnsupported", err)
	}
}
