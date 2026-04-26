package sysinfo

import (
	"errors"
	"os"
	"os/user"
	"strconv"

	"golang.org/x/sys/unix"
)

type ResolvedUser struct {
	UID  uint32
	Name string
	Home string
}

type SudoUserResolver struct {
	LookupByUID  func(uid uint32) (name, home string, err error)
	LookupByName func(name string) (uid uint32, home string, err error)
	Lstat        func(path string) (uid uint32, isSymlink bool, err error)
}

// DefaultSudoUserResolver returns a SudoUserResolver using system calls for user lookups.
// DefaultSudoUserResolver 返回使用系统调用进行用户查找的 SudoUserResolver。
func DefaultSudoUserResolver() SudoUserResolver {
	return SudoUserResolver{
		LookupByUID: func(uid uint32) (string, string, error) {
			u, err := user.LookupId(strconv.FormatUint(uint64(uid), 10))
			if err != nil {
				return "", "", err
			}
			return u.Username, u.HomeDir, nil
		},
		LookupByName: func(name string) (uint32, string, error) {
			u, err := user.Lookup(name)
			if err != nil {
				return 0, "", err
			}
			id, err := strconv.ParseUint(u.Uid, 10, 32)
			if err != nil {
				return 0, "", err
			}
			return uint32(id), u.HomeDir, nil
		},
		Lstat: func(path string) (uint32, bool, error) {
			var st unix.Stat_t
			if err := unix.Lstat(path, &st); err != nil {
				return 0, false, err
			}
			return st.Uid, (st.Mode & unix.S_IFMT) == unix.S_IFLNK, nil
		},
	}
}

func (r SudoUserResolver) Resolve(env map[string]string) (ResolvedUser, error) {
	uidStr := env["SUDO_UID"]
	name := env["SUDO_USER"]
	if uidStr == "" || name == "" {
		return ResolvedUser{}, errors.New("SUDO_UID or SUDO_USER not set")
	}
	uid64, err := strconv.ParseUint(uidStr, 10, 32)
	if err != nil || uid64 == 0 {
		return ResolvedUser{}, errors.New("SUDO_UID invalid or zero")
	}
	uid := uint32(uid64)
	gotName, homeByUID, err := r.LookupByUID(uid)
	if err != nil {
		return ResolvedUser{}, err
	}
	uidByName, homeByName, err := r.LookupByName(name)
	if err != nil {
		return ResolvedUser{}, err
	}
	if gotName != name || uidByName != uid || homeByUID != homeByName {
		return ResolvedUser{}, errors.New("SUDO_UID and SUDO_USER disagree")
	}
	owner, isSymlink, err := r.Lstat(homeByUID)
	if err != nil {
		return ResolvedUser{}, err
	}
	if isSymlink || owner != uid {
		return ResolvedUser{}, errors.New("home directory ownership/symlink check failed")
	}
	return ResolvedUser{UID: uid, Name: name, Home: homeByUID}, nil
}

func EnvMap() map[string]string {
	out := make(map[string]string, 4)
	for _, key := range []string{"SUDO_UID", "SUDO_USER", "HOME", "USER"} {
		out[key] = os.Getenv(key)
	}
	return out
}
