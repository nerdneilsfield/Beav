package executor

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/dengqi/beav/internal/cleaner/safety"
	"golang.org/x/sys/unix"
)

type managedJournalRecord struct {
	Profile       string `json:"profile"`
	Root          string `json:"root"`
	OriginalLeaf  string `json:"original_leaf"`
	TombstoneLeaf string `json:"tombstone_leaf"`
	UID           uint32 `json:"uid"`
	Dev           uint64 `json:"dev"`
	Inode         uint64 `json:"inode"`
	MountID       uint64 `json:"mount_id"`
	Complete      bool   `json:"complete"`
}

type managedJournal struct {
	fd int
}

func (j *managedJournal) Close() error {
	if j == nil || j.fd < 0 {
		return nil
	}
	err := unix.Close(j.fd)
	j.fd = -1
	return err
}

func (j *managedJournal) append(record managedJournalRecord) error {
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	for len(data) > 0 {
		n, writeErr := unix.Write(j.fd, data)
		if writeErr != nil {
			return writeErr
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return unix.Fsync(j.fd)
}

func (e *ManagedCacheExecutor) openManagedJournal() (*managedJournal, error) {
	path := filepath.Clean(e.journalPath)
	if !filepath.IsAbs(path) || !managedPathWithin(path, filepath.Clean(e.Home)) {
		return nil, fmt.Errorf("journal path outside target home")
	}
	parentFD, err := openManagedDirTree(filepath.Clean(e.Home), filepath.Dir(path), e.TargetUID, e.journalDirSync)
	if err != nil {
		return nil, err
	}
	leaf := filepath.Base(path)
	fd, err := unix.Openat(parentFD, leaf, unix.O_RDWR|unix.O_APPEND|unix.O_CREAT|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err == nil {
		if e.journalParentSync == nil {
			err = errors.New("journal parent fsync unavailable")
		} else {
			err = e.journalParentSync(parentFD)
		}
	}
	_ = unix.Close(parentFD)
	if err != nil {
		if fd >= 0 {
			_ = unix.Close(fd)
		}
		return nil, err
	}
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	if st.Uid != e.TargetUID || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Mode&0o077 != 0 {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("journal ownership or mode is not trusted")
	}
	return &managedJournal{fd: fd}, nil
}

func loadManagedJournal(fd int) ([]managedJournalRecord, error) {
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return nil, err
	}
	if st.Size == 0 {
		return nil, nil
	}
	data := make([]byte, st.Size)
	read := 0
	for read < len(data) {
		n, err := unix.Pread(fd, data[read:], int64(read))
		if err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, io.ErrUnexpectedEOF
		}
		read += n
	}
	// A record is trusted only after its newline delimiter was persisted. Drop
	// a final unterminated line, preserving earlier complete records.
	if len(data) > 0 && data[len(data)-1] != '\n' {
		if last := bytes.LastIndexByte(data, '\n'); last >= 0 {
			data = data[:last+1]
		} else {
			data = nil
		}
	}
	if len(data) == 0 {
		return nil, nil
	}
	var records []managedJournalRecord
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var record managedJournalRecord
		if err := json.Unmarshal(line, &record); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	latest := make(map[string]managedJournalRecord)
	order := make([]string, 0, len(records))
	for _, record := range records {
		key := managedJournalKey(record)
		if _, exists := latest[key]; !exists {
			order = append(order, key)
		}
		latest[key] = record
	}
	out := make([]managedJournalRecord, 0, len(order))
	for _, key := range order {
		out = append(out, latest[key])
	}
	return out, nil
}

func managedJournalKey(record managedJournalRecord) string {
	return strings.Join([]string{record.Profile, record.Root, record.OriginalLeaf, record.TombstoneLeaf}, "\x00")
}

func openManagedDirTree(home, target string, uid uint32, syncParent func(int) error) (int, error) {
	home = filepath.Clean(home)
	target = filepath.Clean(target)
	if !managedPathWithin(target, home) {
		return -1, safety.ErrNotInsideRoot
	}
	cur, err := safety.OpenAnchoredDirFD(home, home)
	if err != nil {
		return -1, err
	}
	rel, err := filepath.Rel(home, target)
	if err != nil {
		_ = unix.Close(cur)
		return -1, err
	}
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if !safeManagedLeaf(part) {
			_ = unix.Close(cur)
			return -1, safety.ErrNotDir
		}
		var st unix.Stat_t
		statErr := unix.Fstatat(cur, part, &st, unix.AT_SYMLINK_NOFOLLOW)
		if errors.Is(statErr, unix.ENOENT) {
			mkdirErr := unix.Mkdirat(cur, part, 0o700)
			if mkdirErr != nil && !errors.Is(mkdirErr, unix.EEXIST) {
				_ = unix.Close(cur)
				return -1, mkdirErr
			}
			if mkdirErr == nil {
				if syncParent == nil {
					_ = unix.Close(cur)
					return -1, errors.New("journal directory fsync unavailable")
				}
				if err := syncParent(cur); err != nil {
					_ = unix.Close(cur)
					return -1, err
				}
			}
			statErr = unix.Fstatat(cur, part, &st, unix.AT_SYMLINK_NOFOLLOW)
		}
		if statErr != nil {
			_ = unix.Close(cur)
			return -1, statErr
		}
		if st.Mode&unix.S_IFMT != unix.S_IFDIR || st.Uid != uid {
			_ = unix.Close(cur)
			return -1, errors.New("journal directory is not target-owned")
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
