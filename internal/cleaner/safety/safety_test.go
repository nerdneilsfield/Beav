package safety

import "testing"

// TestInsideAllowList verifies that paths are correctly classified as inside or outside the allow list.
// TestInsideAllowList 验证路径是否正确分类为在允许列表内或外。
func TestInsideAllowList(t *testing.T) {
	cases := []struct {
		p  string
		ok bool
	}{
		{"/home/u/.cache/x", true},
		{"/var/cache/apt", true},
		{"/var/log/journal", true},
		{"/tmp/foo", true},
		{"/var/tmp/foo", true},
		{"/etc/passwd", false},
		{"/usr/bin/x", false},
	}
	for _, c := range cases {
		if got := InsideAllowList(c.p, "/home/u"); got != c.ok {
			t.Errorf("InsideAllowList(%q) = %v want %v", c.p, got, c.ok)
		}
	}
}

// TestBlacklisted verifies that blacklisted paths are correctly identified.
// TestBlacklisted 验证黑名单路径是否正确识别。
func TestBlacklisted(t *testing.T) {
	home := "/home/u"
	for _, p := range []string{
		"/", "/etc", "/boot", "/usr", "/usr/lib",
		"/home/other/.cache",
		"/home/u",
		"/home/u/Documents", "/home/u/.ssh", "/home/u/.gnupg",
		"/home/u/.docker/config.json", "/home/u/.kube/config",
		"/var/lib/docker", "/var/lib/containerd", "/var/lib/kubelet",
	} {
		if !Blacklisted(p, home) {
			t.Errorf("expected %q blacklisted", p)
		}
	}
	for _, p := range []string{
		"/home/u/.cache/x",
		"/home/u/.kube/cache/x",
		"/home/u/.kube/http-cache/y",
	} {
		if Blacklisted(p, home) {
			t.Errorf("expected %q not blacklisted", p)
		}
	}
}

// TestBlacklistRejectsKernelVirtualFilesystems verifies that kernel virtual filesystems are blacklisted.
// TestBlacklistRejectsKernelVirtualFilesystems 验证内核虚拟文件系统被加入黑名单。
func TestBlacklistRejectsKernelVirtualFilesystems(t *testing.T) {
	for _, p := range []string{"/proc", "/proc/self", "/sys", "/sys/kernel", "/dev", "/dev/null", "/run", "/run/user"} {
		if !Blacklisted(p, "/home/u") {
			t.Fatalf("%s should be blacklisted", p)
		}
	}
}
