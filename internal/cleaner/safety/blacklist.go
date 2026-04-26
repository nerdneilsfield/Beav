package safety

import (
	"path/filepath"
	"strings"
)

// hardDenyExact contains absolute paths that are strictly forbidden from any cleaning operation.
// hardDenyExact 包含在任何清理操作中都被严格禁止的绝对路径。
var hardDenyExact = []string{"/", "/etc", "/boot", "/usr", "/proc", "/sys", "/dev", "/run"}

// hardDenyPrefix contains path prefixes that must never be cleaned, including their subdirectories.
// hardDenyPrefix 包含绝不能被清理的路径前缀，包括其子目录。
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

// homeBlacklistRel contains relative paths under the home directory that must be protected.
// homeBlacklistRel 包含必须保护的主目录下的相对路径。
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

// Blacklisted checks whether a given path is on the blacklist and must not be cleaned.
// Blacklisted 检查给定路径是否在黑名单中且不得被清理。
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
