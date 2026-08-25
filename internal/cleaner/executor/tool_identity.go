package executor

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/dengqi/beav/internal/cleaner/model"
	"golang.org/x/sys/unix"
)

func (e *ToolPruneExecutor) identityFor(c model.Cleaner, profile toolProfile) (CommandIdentity, error) {
	if profile.System {
		if os.Geteuid() != 0 {
			return CommandIdentity{}, errors.New("system profile requires root")
		}
		return CommandIdentity{UID: 0, GID: 0, Groups: []uint32{}, Name: "root", Home: "/root"}, nil
	}
	if e.Home == "" || !filepath.IsAbs(e.Home) || c.Scope != model.ScopeUser {
		return CommandIdentity{}, errors.New("user profile requires target home")
	}
	if e.HasTarget {
		if e.Target.UID == 0 {
			return CommandIdentity{}, errors.New("target UID must be non-zero")
		}
		if os.Geteuid() != 0 && e.Target.UID != systemID(os.Getuid()) {
			return CommandIdentity{}, errors.New("non-root process cannot target another UID")
		}
		// Target GID, groups, name, and home are mutable caller input. Resolve
		// every one from NSS; only the UID selects account.
		identity, err := e.resolveUserIdentity(e.Target.UID, "")
		if err != nil {
			return CommandIdentity{}, err
		}
		return identity, nil
	}
	uid := systemID(os.Getuid())
	if uid == 0 {
		// A root process cleaning --all must carry an explicitly resolved target
		// identity. Falling back to root here would violate the sudo boundary.
		sudoUID := os.Getenv("SUDO_UID")
		sudoUser := os.Getenv("SUDO_USER")
		if sudoUID == "" || sudoUser == "" {
			return CommandIdentity{}, errors.New("target identity required for root user-scope tool")
		}
		parsed, parseErr := strconv.ParseUint(sudoUID, 10, 32)
		if parseErr != nil || parsed == 0 {
			return CommandIdentity{}, errors.New("SUDO_UID invalid or zero")
		}
		if targetUser, lookupErr := user.LookupId(strconv.FormatUint(parsed, 10)); lookupErr != nil || filepath.Clean(targetUser.HomeDir) != filepath.Clean(e.Home) {
			return CommandIdentity{}, errors.New("target home does not match SUDO target")
		}
		identity, lookupErr := lookupIdentity(uint32(parsed), sudoUser, e.Home)
		if lookupErr != nil {
			return CommandIdentity{}, lookupErr
		}
		return identity, nil
	}
	identity, err := e.resolveUserIdentity(uid, "")
	if err != nil {
		return CommandIdentity{}, err
	}
	return identity, nil
}

func (e *ToolPruneExecutor) resolveUserIdentity(uid uint32, expectedName string) (CommandIdentity, error) {
	if e.identityResolver != nil {
		identity, err := e.identityResolver(uid)
		if err != nil {
			return CommandIdentity{}, err
		}
		if identity.UID != uid {
			return CommandIdentity{}, errors.New("injected identity UID does not match requested UID")
		}
		if expectedName != "" && identity.Name != expectedName {
			return CommandIdentity{}, errors.New("target username does not match SUDO_USER")
		}
		if err := validateResolvedIdentity(identity); err != nil {
			return CommandIdentity{}, err
		}
		return identity, nil
	}
	return lookupIdentity(uid, expectedName, e.Home)
}

func validateResolvedIdentity(identity CommandIdentity) error {
	if identity.UID == 0 {
		return errors.New("target UID must be non-zero")
	}
	if identity.GID == 0 {
		return errors.New("target primary GID must be non-zero")
	}
	if identity.Name == "" {
		return errors.New("target username is empty")
	}
	for _, gid := range identity.Groups {
		if gid == 0 {
			return errors.New("target supplementary GID must be non-zero")
		}
		if gid == identity.GID {
			return errors.New("target primary GID leaked into supplementary groups")
		}
	}
	if err := verifyTargetHome(identity); err != nil {
		return err
	}
	if err := ensureCurrentIdentity(identity); err != nil {
		return err
	}
	return nil
}

func ensureCurrentIdentity(identity CommandIdentity) error {
	if os.Geteuid() == 0 {
		return nil
	}
	if systemID(os.Getuid()) != identity.UID || systemID(os.Getgid()) != identity.GID {
		return errors.New("current process credentials do not match target identity")
	}
	groups, err := unix.Getgroups()
	if err != nil {
		return fmt.Errorf("read current supplementary groups: %w", err)
	}
	actual := make([]uint32, len(groups))
	actual = actual[:0]
	for _, gid := range groups {
		if gid < 0 {
			return errors.New("current supplementary GID is invalid")
		}
		if systemID(gid) != identity.GID {
			actual = append(actual, systemID(gid))
		}
	}
	sort.Slice(actual, func(i, j int) bool { return actual[i] < actual[j] })
	want := append([]uint32(nil), identity.Groups...)
	sort.Slice(want, func(i, j int) bool { return want[i] < want[j] })
	if len(actual) != len(want) {
		return fmt.Errorf("current supplementary groups do not match target identity: got %v want %v", actual, want)
	}
	for i := range actual {
		if actual[i] != want[i] {
			return fmt.Errorf("current supplementary groups do not match target identity: got %v want %v", actual, want)
		}
	}
	return nil
}

func systemID(id int) uint32 {
	return uint32(id) //nolint:gosec // Linux UID/GID values originate from uint32 kernel IDs.
}

func lookupIdentity(uid uint32, expectedName, home string) (CommandIdentity, error) {
	if uid == 0 {
		return CommandIdentity{}, errors.New("target UID must be non-zero")
	}
	u, err := user.LookupId(strconv.FormatUint(uint64(uid), 10))
	if err != nil {
		return CommandIdentity{}, err
	}
	if expectedName != "" && u.Username != expectedName {
		return CommandIdentity{}, errors.New("target username does not match SUDO_USER")
	}
	gid64, err := strconv.ParseUint(u.Gid, 10, 32)
	if err != nil {
		return CommandIdentity{}, err
	}
	if gid64 == 0 {
		return CommandIdentity{}, errors.New("target primary GID must be non-zero")
	}
	groups, err := u.GroupIds()
	if err != nil {
		return CommandIdentity{}, err
	}
	groupIDs := make([]uint32, 0, len(groups))
	for _, raw := range groups {
		gid, parseErr := strconv.ParseUint(raw, 10, 32)
		if parseErr != nil {
			return CommandIdentity{}, fmt.Errorf("invalid supplementary GID %q: %w", raw, parseErr)
		}
		if gid == 0 {
			return CommandIdentity{}, errors.New("target supplementary GID must be non-zero")
		}
		if uint32(gid) == uint32(gid64) {
			continue
		}
		groupIDs = append(groupIDs, uint32(gid))
	}
	canonicalHome := filepath.Clean(u.HomeDir)
	if !filepath.IsAbs(canonicalHome) {
		return CommandIdentity{}, errors.New("NSS home is not absolute")
	}
	if home != "" && filepath.Clean(home) != canonicalHome {
		return CommandIdentity{}, errors.New("target home does not match authoritative NSS HomeDir")
	}
	identity := normalizeIdentity(CommandIdentity{UID: uid, GID: uint32(gid64), Groups: groupIDs, Name: u.Username, Home: canonicalHome})
	if err := verifyTargetHome(identity); err != nil {
		return CommandIdentity{}, err
	}
	return identity, nil
}

func verifyTargetHome(identity CommandIdentity) error {
	if identity.Home == "" {
		return errors.New("target home is empty")
	}
	var st unix.Stat_t
	if err := unix.Lstat(identity.Home, &st); err != nil {
		return err
	}
	if st.Mode&unix.S_IFMT == unix.S_IFLNK || st.Mode&unix.S_IFMT != unix.S_IFDIR {
		return errors.New("target home is not a directory")
	}
	if st.Uid != identity.UID {
		return errors.New("target home is not target-owned")
	}
	return nil
}

func normalizeIdentity(identity CommandIdentity) CommandIdentity {
	seen := map[uint32]bool{}
	groups := make([]uint32, 0, len(identity.Groups))
	for _, gid := range identity.Groups {
		if gid == 0 || gid == identity.GID || seen[gid] {
			continue
		}
		seen[gid] = true
		groups = append(groups, gid)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i] < groups[j] })
	identity.Groups = groups
	return identity
}

func (e *ToolPruneExecutor) environment(identity CommandIdentity, profile toolProfile, searchPath []string) []string {
	home := identity.Home
	lang := os.Getenv("LANG")
	if lang == "" {
		lang = "C.UTF-8"
	}
	values := map[string]string{
		"HOME":            home,
		"USER":            identity.Name,
		"LOGNAME":         identity.Name,
		"XDG_CACHE_HOME":  filepath.Join(home, ".cache"),
		"XDG_CONFIG_HOME": filepath.Join(home, ".config"),
		"XDG_DATA_HOME":   filepath.Join(home, ".local", "share"),
		"XDG_STATE_HOME":  filepath.Join(home, ".local", "state"),
		"XDG_RUNTIME_DIR": filepath.Join("/run/user", strconv.FormatUint(uint64(identity.UID), 10)),
		"TMPDIR":          "/tmp",
		"PATH":            strings.Join(searchPath, string(os.PathListSeparator)),
		"LANG":            lang,
	}
	for key, value := range profile.ExtraEnv {
		values[key] = value
	}
	for key, value := range e.ExtraEnv {
		if _, reserved := values[key]; !reserved {
			values[key] = value
		}
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	env := make([]string, 0, len(keys))
	for _, key := range keys {
		env = append(env, key+"="+values[key])
	}
	return env
}

func (e *ToolPruneExecutor) executionIdentity(identity CommandIdentity) *CommandIdentity {
	// A non-root process cannot call setgroups, even with its current group
	// list. It already has the desired identity, so avoid a failing redundant
	// credential transition. sudo --all runs as root and takes this path.
	if os.Geteuid() != 0 {
		return nil
	}
	return &identity
}
