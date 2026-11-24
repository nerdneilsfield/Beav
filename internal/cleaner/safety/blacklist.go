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

// DefaultSystemDenyPrefixes returns a copy of built-in protected system path prefixes.
// DefaultSystemDenyPrefixes 返回内置受保护系统路径前缀的副本。
func DefaultSystemDenyPrefixes() []string {
	out := make([]string, len(hardDenyExact))
	copy(out, hardDenyExact)
	for _, p := range hardDenyPrefix {
		p = strings.TrimSuffix(p, "/")
		if !containsStr(out, p) {
			out = append(out, p)
		}
	}
	return out
}

// DefaultHomeDenyRel returns a copy of built-in protected home-relative paths.
// DefaultHomeDenyRel 返回内置受保护主目录相对路径的副本。
func DefaultHomeDenyRel() []string {
	out := make([]string, len(homeBlacklistRel))
	copy(out, homeBlacklistRel)
	return out
}

func containsStr(list []string, target string) bool {
	for _, item := range list {
		if item == target {
			return true
		}
	}
	return false
}

// Blacklisted checks whether a given path is on the default blacklist and must not be cleaned.
// Blacklisted 检查给定路径是否在默认黑名单中且不得被清理。
func Blacklisted(path, home string) bool {
	return BlacklistedWithPolicy(path, home, DefaultPolicy())
}

// BlacklistedWithPolicy checks whether path is protected under the given Policy.
// Literal "/" is always protected to prevent catastrophic wildcard/env accidents.
// BlacklistedWithPolicy 根据给定 Policy 检查路径是否受保护；根目录 "/" 本身始终受保护。
func BlacklistedWithPolicy(path, home string, policy Policy) bool {
	clean := filepath.Clean(path)
	if clean == "/" {
		return true
	}
	if policy.EffectiveDisableBlacklist() {
		return false
	}

	for _, entry := range policy.Blacklist {
		exp := ExpandPath(entry, home)
		if exp == "" {
			continue
		}
		if hasPrefix(clean, exp) {
			return true
		}
	}

	if policy.ReplaceBlacklist {
		return false
	}

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
