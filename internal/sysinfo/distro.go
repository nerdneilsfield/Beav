package sysinfo

import (
	"bufio"
	"os"
	"os/exec"
	"strings"
)

type OSRelease struct {
	ID     string
	IDLike string
}

func DetectOSRelease() OSRelease {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return OSRelease{}
	}
	return parseOSRelease(string(data))
}

func DetectPackageManager() (string, bool) {
	return detectPackageManager(DetectOSRelease(), func(name string) bool {
		_, err := exec.LookPath(name)
		return err == nil
	})
}

func parseOSRelease(s string) OSRelease {
	out := OSRelease{}
	scanner := bufio.NewScanner(strings.NewReader(s))
	for scanner.Scan() {
		line := scanner.Text()
		if k, v, ok := strings.Cut(line, "="); ok {
			switch k {
			case "ID":
				out.ID = trimOSRelease(v)
			case "ID_LIKE":
				out.IDLike = trimOSRelease(v)
			}
		}
	}
	return out
}

func trimOSRelease(s string) string {
	return strings.Trim(s, `"'`)
}

func detectPackageManager(osr OSRelease, hasBinary func(string) bool) (string, bool) {
	preferred := distroPackageManagers(osr)
	for _, candidate := range preferred {
		if hasBinary(packageManagerBinary(candidate)) {
			return candidate, true
		}
	}
	for _, candidate := range []string{"apt", "dnf", "pacman", "zypper"} {
		if hasBinary(packageManagerBinary(candidate)) {
			return candidate, true
		}
	}
	return "", false
}

func distroPackageManagers(osr OSRelease) []string {
	ids := strings.Fields(osr.ID + " " + osr.IDLike)
	for _, id := range ids {
		switch id {
		case "debian", "ubuntu":
			return []string{"apt"}
		case "fedora", "rhel", "centos":
			return []string{"dnf"}
		case "arch", "archlinux":
			return []string{"pacman"}
		case "opensuse", "suse", "sles":
			return []string{"zypper"}
		}
	}
	return nil
}

func packageManagerBinary(manager string) string {
	if manager == "apt" {
		return "apt-get"
	}
	return manager
}
