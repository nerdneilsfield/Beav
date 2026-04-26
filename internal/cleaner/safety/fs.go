package safety

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/sys/unix"
)

// Action represents the operation to perform on a filesystem entry.
// Action 表示对文件系统条目执行的操作。
type Action int

const (
	// ActionDelete indicates the entry should be deleted.
	// ActionDelete 表示该条目应被删除。
	ActionDelete Action = iota
	// ActionSkip indicates the entry should be skipped.
	// ActionSkip 表示该条目应被跳过。
	ActionSkip
)

// ErrChanged is returned when an entry has been modified since it was last stat'd.
// ErrChanged 在条目自上次 stat 后被修改时返回。
var ErrChanged = errors.New("entry changed since stat")

// ErrCrossFS is returned when an entry resides on a different filesystem.
// ErrCrossFS 在条目位于不同文件系统时返回。
var ErrCrossFS = errors.New("entry on different filesystem")

// ErrNotFound is returned when an entry cannot be located.
// ErrNotFound 在无法找到条目时返回。
var ErrNotFound = errors.New("entry not found")

// ErrSymlink is returned when a path component is a symbolic link.
// ErrSymlink 在路径组件为符号链接时返回。
var ErrSymlink = errors.New("path component is a symlink")

// ErrNotInsideRoot is returned when a target path is not under the safe root directory.
// ErrNotInsideRoot 在目标路径不在安全根目录下时返回。
var ErrNotInsideRoot = errors.New("target is not under the safe root")

// ErrNotDir is returned when a path component is not a directory.
// ErrNotDir 在路径组件不是目录时返回。
var ErrNotDir = errors.New("path component is not a directory")

// Entry represents a filesystem entry with its relative path and stat information.
// Entry 表示一个文件系统条目，包含其相对路径和 stat 信息。
type Entry struct {
	// RelPath is the path relative to the walker root.
	// RelPath 是相对于 walker 根目录的路径。
	RelPath string
	stat    unix.Stat_t
}

// IsRegular returns true if the entry is a regular file.
// IsRegular 如果条目是普通文件则返回 true。
func (e Entry) IsRegular() bool { return (e.stat.Mode & unix.S_IFMT) == unix.S_IFREG }

// IsDir returns true if the entry is a directory.
// IsDir 如果条目是目录则返回 true。
func (e Entry) IsDir() bool     { return (e.stat.Mode & unix.S_IFMT) == unix.S_IFDIR }

// IsSymlink returns true if the entry is a symbolic link.
// IsSymlink 如果条目是符号链接则返回 true。
func (e Entry) IsSymlink() bool { return (e.stat.Mode & unix.S_IFMT) == unix.S_IFLNK }

// Size returns the size of the entry in bytes.
// Size 返回条目的大小（以字节为单位）。
func (e Entry) Size() int64     { return e.stat.Size }

// Mtime returns the modification time of the entry as a Unix timestamp.
// Mtime 返回条目的修改时间（Unix 时间戳）。
func (e Entry) Mtime() int64    { return int64(e.stat.Mtim.Sec) }

// Ctime returns the change time of the entry as a Unix timestamp.
// Ctime 返回条目的状态变更时间（Unix 时间戳）。
func (e Entry) Ctime() int64    { return int64(e.stat.Ctim.Sec) }

// Walker provides safe filesystem traversal anchored at a root directory.
// Walker 提供以根目录为锚点的安全文件系统遍历功能。
type Walker struct {
	rootFD  int
	rootDev uint64
	rootAbs string
}

// OpenWalker opens a new Walker rooted at the given directory path.
// OpenWalker 打开一个以给定目录路径为根的新 Walker。
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

// OpenWalkerFD opens a new Walker using an existing directory file descriptor.
// OpenWalkerFD 使用现有的目录文件描述符打开一个新的 Walker。
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

// Close releases the file descriptor held by the Walker.
// Close 释放 Walker 持有的文件描述符。
func (w *Walker) Close() error { return unix.Close(w.rootFD) }

// Root returns the absolute path of the walker root directory.
// Root 返回 walker 根目录的绝对路径。
func (w *Walker) Root() string { return w.rootAbs }

// WalkFunc is the callback function type invoked for each entry during Walk.
// WalkFunc 是在 Walk 期间为每个条目调用的回调函数类型。
type WalkFunc func(Entry)

// Walk traverses the filesystem tree and calls fn for each entry.
// Walk 遍历文件系统树并为每个条目调用 fn。
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
	// Treat a directory that directly contains .git as a repository root and
	// skip that whole subtree; the parent still continues with other entries.
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

// UnlinkIfUnchanged removes a file only if it has not been modified since the Entry was captured.
// UnlinkIfUnchanged 仅在文件自捕获 Entry 以来未被修改时才删除它。
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

// RemoveEmptyDirIfMatch removes an empty directory only if it matches the original Entry.
// RemoveEmptyDirIfMatch 仅在空目录与原始 Entry 匹配时才删除它。
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

// OpenFileEntry opens a regular file and returns its Walker and Entry for safe operations.
// OpenFileEntry 打开一个普通文件并返回其 Walker 和 Entry 以进行安全操作。
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

// OpenAnchoredDirFD safely opens a directory path anchored within safeRoot, rejecting symlinks and cross-fs traversals.
// OpenAnchoredDirFD 安全地打开锚定在 safeRoot 内的目录路径，拒绝符号链接和跨文件系统遍历。
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
			return -1, ErrNotDir
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
			return -1, ErrNotDir
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

// OpenAnchoredFile safely opens a file anchored within safeRoot, rejecting symlinks and cross-fs traversals.
// OpenAnchoredFile 安全地打开锚定在 safeRoot 内的文件，拒绝符号链接和跨文件系统遍历。
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
