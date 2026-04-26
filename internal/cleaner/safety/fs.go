package safety

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/sys/unix"
)

type Action int

const (
	ActionDelete Action = iota
	ActionSkip
)

var ErrChanged = errors.New("entry changed since stat")
var ErrCrossFS = errors.New("entry on different filesystem")
var ErrNotFound = errors.New("entry not found")
var ErrSymlink = errors.New("path component is a symlink")
var ErrNotInsideRoot = errors.New("target is not under the safe root")

type Entry struct {
	RelPath string
	stat    unix.Stat_t
}

func (e Entry) IsRegular() bool { return (e.stat.Mode & unix.S_IFMT) == unix.S_IFREG }
func (e Entry) IsDir() bool     { return (e.stat.Mode & unix.S_IFMT) == unix.S_IFDIR }
func (e Entry) IsSymlink() bool { return (e.stat.Mode & unix.S_IFMT) == unix.S_IFLNK }
func (e Entry) Size() int64     { return e.stat.Size }
func (e Entry) Mtime() int64    { return int64(e.stat.Mtim.Sec) }
func (e Entry) Ctime() int64    { return int64(e.stat.Ctim.Sec) }

type Walker struct {
	rootFD  int
	rootDev uint64
	rootAbs string
}

func OpenWalker(root string) (*Walker, error) {
	fd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	abs, _ := filepath.Abs(root)
	return &Walker{rootFD: fd, rootDev: st.Dev, rootAbs: abs}, nil
}

func OpenWalkerFD(fd int, absPath string) (*Walker, error) {
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	if (st.Mode & unix.S_IFMT) != unix.S_IFDIR {
		_ = unix.Close(fd)
		return nil, errors.New("fd is not a directory")
	}
	return &Walker{rootFD: fd, rootDev: st.Dev, rootAbs: absPath}, nil
}

func (w *Walker) Close() error { return unix.Close(w.rootFD) }
func (w *Walker) Root() string { return w.rootAbs }

type WalkFunc func(Entry)

func (w *Walker) Walk(fn WalkFunc) error {
	_, err := w.walkDir(w.rootFD, "", fn)
	return err
}

func (w *Walker) walkDir(parentFD int, rel string, fn WalkFunc) (bool, error) {
	names, err := readdirnames(parentFD)
	if err != nil {
		return false, err
	}
	sort.Strings(names)
	for _, n := range names {
		if n == ".git" {
			return true, nil
		}
	}

	for _, name := range names {
		var st unix.Stat_t
		if err := unix.Fstatat(parentFD, name, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			continue
		}
		if st.Dev != w.rootDev {
			continue
		}
		entryRel := filepath.Join(rel, name)
		mode := st.Mode & unix.S_IFMT
		switch mode {
		case unix.S_IFLNK:
			continue
		case unix.S_IFREG, unix.S_IFDIR:
		default:
			continue
		}
		e := Entry{RelPath: entryRel, stat: st}
		if e.IsDir() {
			fd, err := unix.Openat(parentFD, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			if err != nil {
				continue
			}
			gitTree, walkErr := w.walkDir(fd, entryRel, fn)
			_ = unix.Close(fd)
			if walkErr != nil {
				return false, walkErr
			}
			if gitTree {
				continue
			}
		}
		fn(e)
	}
	return false, nil
}

func (w *Walker) reopenLeaf(rel string) (int, string, error) {
	clean := filepath.Clean(rel)
	if clean == "." || clean == string(filepath.Separator) || strings.HasPrefix(clean, "..") {
		return -1, "", ErrNotFound
	}
	parts := strings.Split(clean, string(filepath.Separator))
	cur, err := unix.Dup(w.rootFD)
	if err != nil {
		return -1, "", err
	}
	for i := 0; i < len(parts)-1; i++ {
		next, err := unix.Openat(cur, parts[i], unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		_ = unix.Close(cur)
		if err != nil {
			return -1, "", err
		}
		cur = next
	}
	return cur, parts[len(parts)-1], nil
}

func (w *Walker) UnlinkIfUnchanged(e Entry) error {
	if e.IsDir() {
		return errors.New("use RemoveEmptyDirIfMatch for directories")
	}
	parentFD, leaf, err := w.reopenLeaf(e.RelPath)
	if err != nil {
		return err
	}
	defer unix.Close(parentFD)

	var cur unix.Stat_t
	if err := unix.Fstatat(parentFD, leaf, &cur, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	if cur.Dev != w.rootDev {
		return ErrCrossFS
	}
	if cur.Ino != e.stat.Ino || cur.Dev != e.stat.Dev || cur.Size != e.stat.Size ||
		cur.Mtim.Sec != e.stat.Mtim.Sec || cur.Mtim.Nsec != e.stat.Mtim.Nsec {
		return ErrChanged
	}
	return unix.Unlinkat(parentFD, leaf, 0)
}

func (w *Walker) RemoveEmptyDirIfMatch(e Entry) error {
	if !e.IsDir() {
		return errors.New("use UnlinkIfUnchanged for non-directories")
	}
	parentFD, leaf, err := w.reopenLeaf(e.RelPath)
	if err != nil {
		return err
	}
	defer unix.Close(parentFD)

	var cur unix.Stat_t
	if err := unix.Fstatat(parentFD, leaf, &cur, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	if cur.Dev != w.rootDev {
		return ErrCrossFS
	}
	if (cur.Mode & unix.S_IFMT) != unix.S_IFDIR {
		return ErrChanged
	}
	if cur.Ino != e.stat.Ino || cur.Dev != e.stat.Dev {
		return ErrChanged
	}
	dfd, err := unix.Openat(parentFD, leaf, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	names, readErr := readdirnames(dfd)
	_ = unix.Close(dfd)
	if readErr != nil {
		return readErr
	}
	if len(names) > 0 {
		return ErrChanged
	}
	return unix.Unlinkat(parentFD, leaf, unix.AT_REMOVEDIR)
}

func OpenFileEntry(path string) (*Walker, Entry, error) {
	abs, _ := filepath.Abs(path)
	parent := filepath.Dir(abs)
	pfd, err := unix.Open(parent, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, Entry{}, err
	}
	var st unix.Stat_t
	if err := unix.Fstatat(pfd, filepath.Base(abs), &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		_ = unix.Close(pfd)
		return nil, Entry{}, err
	}
	if (st.Mode & unix.S_IFMT) != unix.S_IFREG {
		_ = unix.Close(pfd)
		return nil, Entry{}, errors.New("not a regular file")
	}
	return &Walker{rootFD: pfd, rootDev: st.Dev, rootAbs: parent}, Entry{RelPath: filepath.Base(abs), stat: st}, nil
}

func readdirnames(fd int) ([]string, error) {
	dupFD, err := unix.Dup(fd)
	if err != nil {
		return nil, err
	}
	d := os.NewFile(uintptr(dupFD), "<dir>")
	defer d.Close()
	return d.Readdirnames(-1)
}

func OpenAnchoredDirFD(safeRoot, target string) (int, error) {
	safeRoot = filepath.Clean(safeRoot)
	target = filepath.Clean(target)
	if target != safeRoot && !strings.HasPrefix(target, safeRoot+string(filepath.Separator)) {
		return -1, ErrNotInsideRoot
	}

	cur, err := unix.Open(safeRoot, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	var rootSt unix.Stat_t
	if err := unix.Fstat(cur, &rootSt); err != nil {
		_ = unix.Close(cur)
		return -1, err
	}
	if target == safeRoot {
		return cur, nil
	}

	rest := strings.TrimPrefix(target, safeRoot+string(filepath.Separator))
	for _, part := range strings.Split(rest, string(filepath.Separator)) {
		if part == "" || part == "." || part == ".." {
			_ = unix.Close(cur)
			return -1, ErrSymlink
		}
		var st unix.Stat_t
		if err := unix.Fstatat(cur, part, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			_ = unix.Close(cur)
			return -1, err
		}
		if (st.Mode & unix.S_IFMT) == unix.S_IFLNK {
			_ = unix.Close(cur)
			return -1, ErrSymlink
		}
		if st.Dev != rootSt.Dev {
			_ = unix.Close(cur)
			return -1, ErrCrossFS
		}
		if (st.Mode & unix.S_IFMT) != unix.S_IFDIR {
			_ = unix.Close(cur)
			return -1, ErrSymlink
		}
		next, err := unix.Openat(cur, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		_ = unix.Close(cur)
		if err != nil {
			return -1, err
		}
		cur = next
	}
	return cur, nil
}

func OpenAnchoredFile(safeRoot, target string) (*Walker, Entry, error) {
	parent := filepath.Dir(filepath.Clean(target))
	parentFD, err := OpenAnchoredDirFD(safeRoot, parent)
	if err != nil {
		return nil, Entry{}, err
	}
	leaf := filepath.Base(target)
	var st unix.Stat_t
	if err := unix.Fstatat(parentFD, leaf, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		_ = unix.Close(parentFD)
		return nil, Entry{}, err
	}
	mode := st.Mode & unix.S_IFMT
	if mode == unix.S_IFLNK {
		_ = unix.Close(parentFD)
		return nil, Entry{}, ErrSymlink
	}
	if mode != unix.S_IFREG {
		_ = unix.Close(parentFD)
		return nil, Entry{}, errors.New("not a regular file")
	}
	return &Walker{rootFD: parentFD, rootDev: st.Dev, rootAbs: parent}, Entry{RelPath: leaf, stat: st}, nil
}
