package executor

import (
	"path/filepath"
	"strconv"
)

type toolProfile struct {
	Binary             string
	NativeDryRun       bool
	System             bool
	RootResolver       func(home string) []string
	BuildArgs          func(root string, age int, dryRun bool) []string
	Busy               func(root string) (bool, error)
	ProbeArgs          []string
	Required           []string
	DryRunProbeArgs    []string
	DryRunRequired     []string
	DryRunVersionWords []string
	VersionWords       []string
	ExtraEnv           map[string]string
}

var toolProfiles = map[string]toolProfile{
	"pkg-homebrew": {
		Binary:       "brew",
		NativeDryRun: true,
		RootResolver: homeRoot(".cache", "Homebrew"),
		BuildArgs: func(_ string, age int, dry bool) []string {
			args := []string{"cleanup", "--prune=" + strconv.Itoa(age)}
			if dry {
				args = append(args, "--dry-run")
			}
			return args
		},
		VersionWords:    []string{"brew"},
		DryRunProbeArgs: []string{"cleanup", "--help"},
		DryRunRequired:  []string{"--dry-run"},
		ExtraEnv: map[string]string{
			"HOMEBREW_NO_AUTO_UPDATE": "1",
			"HOMEBREW_NO_ANALYTICS":   "1",
			"HOMEBREW_NO_AUTOREMOVE":  "1",
		},
	},
	"lang-uv": {
		Binary:       "uv",
		RootResolver: homeRoot(".cache", "uv"),
		BuildArgs:    func(_ string, _ int, _ bool) []string { return []string{"cache", "prune"} },
		VersionWords: []string{"uv"},
	},
	"lang-pnpm": {
		Binary:       "pnpm",
		RootResolver: homeRoot(".local", "share", "pnpm", "store"),
		BuildArgs:    func(_ string, _ int, _ bool) []string { return []string{"store", "prune"} },
		VersionWords: []string{"pnpm"},
	},
	"dev-pre-commit": {
		Binary:       "pre-commit",
		RootResolver: homeRoot(".cache", "pre-commit"),
		BuildArgs:    func(_ string, _ int, _ bool) []string { return []string{"gc"} },
		VersionWords: []string{"pre-commit"},
	},
	"lang-conda": {
		Binary:       "conda",
		NativeDryRun: true,
		RootResolver: homeRoot(".conda"),
		BuildArgs: func(_ string, _ int, dry bool) []string {
			args := []string{"clean", "--index-cache", "--tarballs", "--logfiles", "--yes"}
			if dry {
				args = append(args, "--dry-run", "--json")
			}
			return args
		},
		DryRunProbeArgs: []string{"clean", "--help"},
		DryRunRequired:  []string{"--dry-run", "--json"},
		VersionWords:    []string{"conda"},
	},
	"test-cypress": {
		Binary:       "cypress",
		RootResolver: homeRoot(".cache", "Cypress"),
		BuildArgs:    func(_ string, _ int, _ bool) []string { return []string{"cache", "prune"} },
		VersionWords: []string{"cypress"},
	},
	"build-ccache": {
		Binary:       "ccache",
		RootResolver: homeRoot(".cache", "ccache"),
		BuildArgs:    func(_ string, _ int, _ bool) []string { return []string{"--cleanup"} },
		VersionWords: []string{"ccache"},
	},
	"ai-huggingface-prune": {
		Binary:       "hf",
		NativeDryRun: true,
		RootResolver: homeRoot(".cache", "huggingface"),
		BuildArgs: func(root string, _ int, dry bool) []string {
			args := []string{"cache", "prune", "--cache-dir", root, "--yes"}
			if dry {
				args = append(args, "--dry-run")
			}
			return args
		},
		Busy:           huggingFaceRootBusy,
		ProbeArgs:      []string{"cache", "prune", "--help"},
		Required:       []string{"--cache-dir", "--yes"},
		DryRunRequired: []string{"--dry-run"},
		VersionWords:   []string{"hf", "huggingface"},
	},
	"dev-mise": {
		Binary:       "mise",
		RootResolver: homeRoot(".cache", "mise"),
		BuildArgs:    func(_ string, _ int, _ bool) []string { return []string{"prune", "--yes"} },
		ProbeArgs:    []string{"prune", "--help"},
		Required:     []string{"--yes"},
		VersionWords: []string{"mise"},
	},
	"lang-corepack": {
		Binary:       "corepack",
		RootResolver: homeRoot(".cache", "node", "corepack"),
		BuildArgs:    func(_ string, _ int, _ bool) []string { return []string{"cache", "clean"} },
		VersionWords: []string{"corepack"},
	},
	"sys-nix-gc": {
		Binary:       "nix-collect-garbage",
		NativeDryRun: true,
		System:       true,
		RootResolver: func(_ string) []string { return []string{"/nix"} },
		BuildArgs: func(_ string, _ int, dry bool) []string {
			if dry {
				return []string{"--dry-run"}
			}
			return nil
		},
		DryRunProbeArgs: []string{"--help"},
		DryRunRequired:  []string{"--dry-run"},
		VersionWords:    []string{"nix"},
	},
	"desktop-flatpak-user": {
		Binary:       "flatpak",
		RootResolver: homeRoot(".local", "share", "flatpak"),
		BuildArgs: func(_ string, _ int, _ bool) []string {
			return []string{"uninstall", "--user", "--unused", "--noninteractive"}
		},
		ProbeArgs:    []string{"uninstall", "--help"},
		Required:     []string{"--noninteractive"},
		VersionWords: []string{"flatpak"},
	},
	"desktop-flatpak-system": {
		Binary:       "flatpak",
		System:       true,
		RootResolver: func(_ string) []string { return []string{"/var/lib/flatpak"} },
		BuildArgs: func(_ string, _ int, _ bool) []string {
			return []string{"uninstall", "--system", "--unused", "--noninteractive"}
		},
		ProbeArgs:    []string{"uninstall", "--help"},
		Required:     []string{"--noninteractive"},
		VersionWords: []string{"flatpak"},
	},
	"lang-poetry-cache": {
		Binary:       "poetry",
		RootResolver: homeRoot(".cache", "pypoetry"),
		BuildArgs: func(_ string, _ int, _ bool) []string {
			return []string{"cache", "clear", "pypi", "--all", "--no-interaction"}
		},
		VersionWords: []string{"poetry"},
	},
	"test-playwright-browsers": {
		Binary:       "playwright",
		RootResolver: homeRoot(".cache", "ms-playwright"),
		BuildArgs:    func(_ string, _ int, _ bool) []string { return []string{"uninstall", "--all"} },
		VersionWords: []string{"playwright"},
	},
}

func homeRoot(parts ...string) func(string) []string {
	return func(home string) []string { return []string{filepath.Join(append([]string{home}, parts...)...)} }
}
