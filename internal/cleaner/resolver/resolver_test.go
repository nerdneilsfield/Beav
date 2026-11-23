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

// TestUnknownResolver verifies that Resolve returns an error for unknown resolver names.
// TestUnknownResolver 验证 Resolve 对未知的解析器名称返回错误。
func TestUnknownResolver(t *testing.T) {
	if _, err := Resolve("nosuch", "/home/u"); err == nil {
		t.Fatal("want error")
	}
}

func TestNewDevResolversUseEnvOrDefault(t *testing.T) {
	cases := []struct {
		name     string
		envVar   string
		defRel   string
		override string
	}{
		{"deno_dir", "DENO_DIR", ".cache/deno", "/home/u/custom-deno"},
		{"sccache_dir", "SCCACHE_DIR", ".cache/sccache", "/home/u/custom-sccache"},
		{"zig_cache", "ZIG_GLOBAL_CACHE_DIR", ".cache/zig", "/home/u/custom-zig"},
	}
	for _, tc := range cases {
		t.Setenv(tc.envVar, "")
		got, err := Resolve(tc.name, "/home/u")
		if err != nil {
			t.Fatalf("%s default err: %v", tc.name, err)
		}
		if got != "/home/u/"+tc.defRel {
			t.Fatalf("%s default = %q, want %q", tc.name, got, "/home/u/"+tc.defRel)
		}

		t.Setenv(tc.envVar, tc.override)
		got, err = Resolve(tc.name, "/home/u")
		if err != nil {
			t.Fatalf("%s override err: %v", tc.name, err)
		}
		if got != tc.override {
			t.Fatalf("%s override = %q, want %q", tc.name, got, tc.override)
		}
	}
}
