package executor

import (
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

var managedCacheProfiles = map[string]managedCacheProfile{
	"ai-huggingface-datasets-arrow": {
		minAgeDays:     managedCacheMinAgeDays,
		kind:           managedUnitDirectory,
		resolveRoot:    (*ManagedCacheExecutor).resolveHuggingFaceDatasetsRoot,
		ownerProcesses: []string{"datasets-cli", "huggingface-cli"},
		complete:       completeManagedArrowUnit,
	},
	"ai-torch-hub": {
		minAgeDays:     managedCacheMinAgeDays,
		kind:           managedUnitFile,
		resolveRoot:    (*ManagedCacheExecutor).resolveTorchCheckpointsRoot,
		ownerProcesses: []string{"torchrun"},
		complete:       completeManagedCheckpoint,
	},
}

func (e *ManagedCacheExecutor) resolveHuggingFaceDatasetsRoot() (string, error) {
	if root, ok := e.homeBoundEnv("HF_DATASETS_CACHE"); ok {
		return filepath.Clean(root), nil
	}
	if root, ok := e.homeBoundEnv("HF_HOME"); ok {
		return filepath.Join(root, "datasets"), nil
	}
	if root, ok := e.homeBoundEnv("XDG_CACHE_HOME"); ok {
		return filepath.Join(root, "huggingface", "datasets"), nil
	}
	return filepath.Join(e.Home, ".cache", "huggingface", "datasets"), nil
}

func (e *ManagedCacheExecutor) resolveTorchCheckpointsRoot() (string, error) {
	if root, ok := e.homeBoundEnv("TORCH_HOME"); ok {
		return filepath.Join(root, "hub", "checkpoints"), nil
	}
	if root, ok := e.homeBoundEnv("XDG_CACHE_HOME"); ok {
		return filepath.Join(root, "torch", "hub", "checkpoints"), nil
	}
	return filepath.Join(e.Home, ".cache", "torch", "hub", "checkpoints"), nil
}

func (e *ManagedCacheExecutor) homeBoundEnv(name string) (string, bool) {
	if e.env == nil {
		return "", false
	}
	value := strings.TrimSpace(e.env(name))
	if value == "" || !filepath.IsAbs(value) {
		return "", false
	}
	value = filepath.Clean(value)
	if !managedPathWithin(value, filepath.Clean(e.Home)) {
		return "", false
	}
	return value, true
}

func completeManagedArrowUnit(fd int, leaf string) (bool, error) {
	if strings.Contains(strings.ToLower(leaf), "incomplete") || strings.HasSuffix(strings.ToLower(leaf), ".lock") {
		return false, nil
	}
	names, err := readManagedDirNames(fd)
	if err != nil {
		return false, err
	}
	if len(names) == 0 {
		return false, nil
	}
	regular := false
	for _, name := range names {
		identity, statErr := managedStatAt(fd, name)
		if statErr != nil {
			return false, statErr
		}
		if identity.Mode == unix.S_IFLNK {
			return false, nil
		}
		if identity.Mode == unix.S_IFREG && identity.Nlink != 1 {
			return false, nil
		}
		lower := strings.ToLower(name)
		if strings.Contains(lower, "incomplete") || strings.HasSuffix(lower, ".lock") || strings.HasSuffix(lower, ".part") || strings.HasSuffix(lower, ".tmp") {
			return false, nil
		}
		if identity.Mode == unix.S_IFREG {
			regular = true
		}
	}
	return regular, nil
}

func completeManagedCheckpoint(fd int, leaf string) (bool, error) {
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return false, err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Nlink != 1 {
		return false, nil
	}
	lower := strings.ToLower(leaf)
	if strings.HasSuffix(lower, ".part") || strings.HasSuffix(lower, ".partial") || strings.HasSuffix(lower, ".incomplete") || strings.HasSuffix(lower, ".tmp") || strings.HasSuffix(lower, ".lock") || strings.HasSuffix(lower, ".download") {
		return false, nil
	}
	return true, nil
}
