package safety

import (
	"path/filepath"
	"strings"
)

var hardDenyExact = []string{"/", "/etc", "/boot", "/usr", "/proc", "/sys", "/dev", "/run"}

var hardDenyPrefix = []string{
	"/etc/",
	"/boot/",
	"/usr/",
	"/proc/",
	"/sys/",
	"/dev/",
	"/run/",
	"/var/lib/docker",
	"/var/lib/containerd",
	"/var/lib/kubelet",
}

var homeBlacklistRel = []string{
	"Documents",
	"Desktop",
	"Downloads",
	"Pictures",
	"Videos",
	"Music",
	".ssh",
	".gnupg",
	".password-store",
}

func Blacklisted(path, home string) bool {
	clean := filepath.Clean(path)

	for _, d := range hardDenyExact {
		if clean == d {
			return true
		}
	}
	for _, p := range hardDenyPrefix {
		p = strings.TrimSuffix(p, "/")
		if clean == p || strings.HasPrefix(clean, p+string(filepath.Separator)) {
			return true
		}
	}

	if strings.HasPrefix(clean, "/home/") && home != "" && !hasPrefix(clean, filepath.Clean(home)) {
		return true
	}
	if home == "" {
		return false
	}

	hc := filepath.Clean(home)
	if clean == hc {
		return true
	}
	for _, rel := range homeBlacklistRel {
		full := filepath.Join(hc, rel)
		if clean == full || strings.HasPrefix(clean, full+string(filepath.Separator)) {
			return true
		}
	}
	if clean == filepath.Join(hc, ".docker", "config.json") {
		return true
	}
	if clean == filepath.Join(hc, ".kube", "config") {
		return true
	}
	kube := filepath.Join(hc, ".kube") + string(filepath.Separator)
	if strings.HasPrefix(clean, kube) {
		rest := strings.TrimPrefix(clean, kube)
		first := strings.SplitN(rest, string(filepath.Separator), 2)[0]
		if first != "cache" && first != "http-cache" {
			return true
		}
	}
	return false
}
