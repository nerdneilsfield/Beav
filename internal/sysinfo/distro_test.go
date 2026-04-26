package sysinfo

import "testing"

func TestParseOSRelease(t *testing.T) {
	got := parseOSRelease(`NAME="Ubuntu"
ID=ubuntu
ID_LIKE=debian
VERSION_ID="24.04"`)
	if got.ID != "ubuntu" || got.IDLike != "debian" {
		t.Errorf("%+v", got)
	}
}

// TestDetectPackageManagerPrefersDistroDefaultWhenPresent verifies that distro-preferred package managers are selected first.
// TestDetectPackageManagerPrefersDistroDefaultWhenPresent 验证优先选择发行版首选的包管理器。
func TestDetectPackageManagerPrefersDistroDefaultWhenPresent(t *testing.T) {
	got, ok := detectPackageManager(OSRelease{ID: "ubuntu", IDLike: "debian"}, func(name string) bool {
		return name == "apt-get" || name == "dnf"
	})
	if !ok || got != "apt" {
		t.Fatalf("got %q, %v; want apt, true", got, ok)
	}
}
