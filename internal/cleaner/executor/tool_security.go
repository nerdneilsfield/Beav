package executor

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dengqi/beav/internal/cleaner/safety"
	"golang.org/x/sys/unix"
)

func (e *ToolPruneExecutor) resolveAndCheckRoots(c model.Cleaner, profile toolProfile, identity CommandIdentity) ([]string, error) {
	checked, err := e.resolveCheckedRoots(c, profile, identity)
	if err != nil {
		return nil, err
	}
	roots := make([]string, 0, len(checked))
	for _, root := range checked {
		roots = append(roots, root.path)
	}
	return roots, nil
}

func (e *ToolPruneExecutor) resolveCheckedRoots(_ model.Cleaner, profile toolProfile, identity CommandIdentity) ([]checkedRoot, error) {
	roots := profile.RootResolver(identity.Home)
	if len(roots) == 0 {
		return nil, errRootUnavailable
	}
	checked := make([]checkedRoot, 0, len(roots))
	for _, root := range roots {
		root = filepath.Clean(root)
		if !filepath.IsAbs(root) {
			return nil, fmt.Errorf("affected root %q is not absolute", root)
		}
		if e.Whitelist != nil && e.Whitelist.Intersects(root) {
			return nil, fmt.Errorf("%w: %s", errKeepListIntersection, root)
		}
		if profile.System {
			if !fixedSystemRoot(profile.Binary, root) {
				return nil, fmt.Errorf("affected root %q is not fixed for profile", root)
			}
		} else {
			if !pathWithin(root, identity.Home) || root == filepath.Clean(identity.Home) || safety.Blacklisted(root, identity.Home) {
				return nil, fmt.Errorf("affected root %q is outside target home", root)
			}
		}
		fd, err := safety.OpenAnchoredDirFD(rootAnchor(identity.Home, profile.System), root)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %v", errRootUnavailable, root, err)
		}
		var st unix.Stat_t
		err = unix.Fstat(fd, &st)
		_ = unix.Close(fd)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %v", errRootUnavailable, root, err)
		}
		if st.Uid != identity.UID && !profile.System {
			return nil, fmt.Errorf("affected root %q is not target-owned", root)
		}
		if profile.System && st.Uid != 0 {
			return nil, fmt.Errorf("affected system root %q is not root-owned", root)
		}
		snapshot, snapshotErr := snapshotPath(root)
		if snapshotErr != nil {
			return nil, fmt.Errorf("%w: %s: %v", errRootUnavailable, root, snapshotErr)
		}
		if snapshot != snapshotFromStat(&st) {
			return nil, fmt.Errorf("%w: affected root %q changed during validation", errRootUnavailable, root)
		}
		if profile.System && snapshot.mode&0o022 != 0 {
			return nil, fmt.Errorf("affected system root %q is group/world writable", root)
		}
		checked = append(checked, checkedRoot{path: root, snapshot: snapshot})
	}
	return checked, nil
}

// AffectedRoots resolves and validates all roots declared by a profile before
// any external command is considered. It is useful to callers that want to
// display the safety decision without executing the profile.
// AffectedRoots 在考虑执行 profile 前解析并验证其声明的全部根。
func (e *ToolPruneExecutor) AffectedRoots(c model.Cleaner) ([]string, error) {
	profile, ok := toolProfiles[c.Profile()]
	if !ok {
		return nil, fmt.Errorf("unknown tool profile %q", c.Profile())
	}
	identity, err := e.identityFor(c, profile)
	if err != nil {
		return nil, err
	}
	return e.resolveAndCheckRoots(c, profile, identity)
}

func rootAnchor(home string, system bool) string {
	if system {
		return "/"
	}
	return filepath.Clean(home)
}

func fixedSystemRoot(binary, root string) bool {
	switch binary {
	case "nix-collect-garbage":
		return root == "/nix"
	case "flatpak":
		return root == "/var/lib/flatpak"
	default:
		return false
	}
}

func pathWithin(path, root string) bool {
	path, root = filepath.Clean(path), filepath.Clean(root)
	return path == root || strings.HasPrefix(path, root+string(filepath.Separator))
}

type pathSnapshot struct {
	dev  uint64
	ino  uint64
	uid  uint32
	mode uint32
}

type checkedRoot struct {
	path     string
	snapshot pathSnapshot
}

func snapshotFromStat(st *unix.Stat_t) pathSnapshot {
	return pathSnapshot{dev: uint64(st.Dev), ino: uint64(st.Ino), uid: st.Uid, mode: st.Mode}
}

func snapshotPath(path string) (pathSnapshot, error) {
	var st unix.Stat_t
	if err := unix.Lstat(path, &st); err != nil {
		return pathSnapshot{}, err
	}
	if st.Mode&unix.S_IFMT == unix.S_IFLNK {
		return pathSnapshot{}, fmt.Errorf("path %q is a symlink", path)
	}
	return snapshotFromStat(&st), nil
}

func (s pathSnapshot) revalidate(path string) error {
	current, err := snapshotPath(path)
	if err != nil {
		return err
	}
	if current != s {
		return fmt.Errorf("path %q changed after validation", path)
	}
	return nil
}

func (e *ToolPruneExecutor) searchPath(profile toolProfile, home string) []string {
	if profile.System {
		return defaultToolSearchPath("", true)
	}
	if len(e.SearchPath) > 0 {
		return cleanSearchPath(e.SearchPath)
	}
	return defaultToolSearchPath(home, profile.System)
}

func defaultToolSearchPath(home string, system bool) []string {
	paths := []string{}
	if !system && home != "" {
		paths = append(paths, filepath.Join(home, ".local", "bin"), filepath.Join(home, ".cargo", "bin"))
	}
	paths = append(paths,
		"/usr/local/bin",
		"/usr/bin",
		"/bin",
		"/usr/local/sbin",
		"/usr/sbin",
		"/sbin",
	)
	return cleanSearchPath(paths)
}

func cleanSearchPath(paths []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		if path == "" {
			continue
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			continue
		}
		abs = filepath.Clean(abs)
		if seen[abs] {
			continue
		}
		seen[abs] = true
		out = append(out, abs)
	}
	return out
}

func resolveProfileBinary(name string, searchPath []string) (string, error) {
	return resolveProfileBinaryWithTrust(name, searchPath, false)
}

func resolveTrustedProfileBinary(name string, searchPath []string) (string, error) {
	return resolveProfileBinaryWithTrust(name, searchPath, true)
}

func resolveProfileBinaryWithTrust(name string, searchPath []string, trusted bool) (string, error) {
	for _, dir := range searchPath {
		candidate := filepath.Join(dir, name)
		info, err := os.Lstat(candidate)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
			continue
		}
		abs, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if trusted {
			if err := validateTrustedPath(abs); err != nil {
				continue
			}
		}
		return abs, nil
	}
	return "", fmt.Errorf("profile binary %q not found", name)
}

func (e *ToolPruneExecutor) resolveBinary(profile toolProfile, homes ...string) (string, error) {
	home := e.Home
	if len(homes) > 0 {
		home = homes[0]
	}
	searchPath := e.searchPath(profile, home)
	if e.binaryResolver != nil {
		return e.binaryResolver(profile, searchPath)
	}
	if profile.System {
		return resolveTrustedProfileBinary(profile.Binary, searchPath)
	}
	return resolveProfileBinary(profile.Binary, searchPath)
}

func (e *ToolPruneExecutor) trustedBinary(profile toolProfile) bool {
	return profile.System && e.binaryResolver == nil
}

func (e *ToolPruneExecutor) validateToolState(profile toolProfile, roots []checkedRoot, binary string, binaryState pathSnapshot) error {
	for _, root := range roots {
		if err := root.snapshot.revalidate(root.path); err != nil {
			return fmt.Errorf("affected root %q changed: %w", root.path, err)
		}
		if profile.System {
			current, err := snapshotPath(root.path)
			if err != nil {
				return fmt.Errorf("affected system root %q unavailable: %w", root.path, err)
			}
			if current.uid != 0 || current.mode&0o022 != 0 {
				return fmt.Errorf("affected system root %q lost trusted ownership/mode", root.path)
			}
		}
	}
	if err := binaryState.revalidate(binary); err != nil {
		return fmt.Errorf("profile binary %q changed: %w", binary, err)
	}
	if e.trustedBinary(profile) {
		if err := validateTrustedPath(binary); err != nil {
			return fmt.Errorf("profile binary %q lost trusted path: %w", binary, err)
		}
	}
	return nil
}

func validateTrustedPath(path string) error {
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		return errors.New("trusted path must be absolute")
	}
	rootInfo, err := os.Lstat(string(filepath.Separator))
	if err != nil {
		return err
	}
	rootStat, ok := rootInfo.Sys().(*syscall.Stat_t)
	if !ok || rootStat.Uid != 0 || rootInfo.Mode()&os.ModeSymlink != 0 || rootInfo.Mode().Perm()&0o022 != 0 || !rootInfo.IsDir() {
		return errors.New("trusted path root is not secure")
	}
	current := string(filepath.Separator)
	for _, part := range strings.Split(strings.TrimPrefix(path, string(filepath.Separator)), string(filepath.Separator)) {
		if part == "" || part == "." || part == ".." {
			return errors.New("trusted path contains invalid component")
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("trusted path component %q is symlink", current)
		}
		if info.Mode().Perm()&0o022 != 0 {
			return fmt.Errorf("trusted path component %q is group/world writable", current)
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat.Uid != 0 {
			return fmt.Errorf("trusted path component %q is not root-owned", current)
		}
		if current != path && !info.IsDir() {
			return fmt.Errorf("trusted path component %q is not a directory", current)
		}
	}
	return nil
}
