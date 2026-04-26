package safety

import "testing"

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

func TestBlacklistRejectsKernelVirtualFilesystems(t *testing.T) {
	for _, p := range []string{"/proc", "/proc/self", "/sys", "/sys/kernel", "/dev", "/dev/null", "/run", "/run/user"} {
		if !Blacklisted(p, "/home/u") {
			t.Fatalf("%s should be blacklisted", p)
		}
	}
}
