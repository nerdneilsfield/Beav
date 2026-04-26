package resolver

import (
	"os"
	"testing"
)

func TestXDGCacheUsesEnvOrDefault(t *testing.T) {
	old := os.Getenv("XDG_CACHE_HOME")
	defer os.Setenv("XDG_CACHE_HOME", old)

	os.Unsetenv("XDG_CACHE_HOME")
	got := MustResolve("xdg_cache", "/home/u")
	if got != "/home/u/.cache" {
		t.Errorf("got %q", got)
	}

	os.Setenv("XDG_CACHE_HOME", "/var/somewhere")
	got = MustResolve("xdg_cache", "/home/u")
	if got != "/var/somewhere" {
		t.Errorf("got %q", got)
	}
}

func TestUnknownResolver(t *testing.T) {
	if _, err := Resolve("nosuch", "/home/u"); err == nil {
		t.Fatal("want error")
	}
}
