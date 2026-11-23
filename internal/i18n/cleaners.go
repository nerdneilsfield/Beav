package i18n

import "github.com/dengqi/beav/internal/cleaner/model"

type cleanerLocale struct {
	NameZH  string
	DescEN  string
	DescZH  string
	HeavyEN string
	HeavyZH string
}

var builtinCleanerLocales = map[string]cleanerLocale{
	// System scope
	"pkg-apt-archives": {
		NameZH: "apt 软件包下载缓存",
		DescEN: "Clean downloaded .deb archives from /var/cache/apt via apt-get clean & autoclean.",
		DescZH: "通过 apt-get clean 与 autoclean 清理 /var/cache/apt 中已下载的 .deb 安装包缓存。",
	},
	"pkg-dnf": {
		NameZH: "dnf 软件包缓存",
		DescEN: "Clean cached repository metadata and downloaded RPM packages via dnf clean all.",
		DescZH: "通过 dnf clean all 清理仓库元数据及已下载的 RPM 安装包缓存。",
	},
	"pkg-pacman": {
		NameZH: "pacman 软件包缓存",
		DescEN: "Remove old uninstalled package tarballs via pacman -Sc while keeping installed versions.",
		DescZH: "通过 pacman -Sc 清理旧版本安装包缓存，保留当前已安装版本。",
	},
	"pkg-zypper": {
		NameZH: "zypper 软件包缓存",
		DescEN: "Clean zypper repository and package caches via zypper clean -a.",
		DescZH: "通过 zypper clean -a 清理 zypper 仓库元数据与安装包缓存。",
	},
	"sys-journal": {
		NameZH: "systemd 系统日志",
		DescEN: "Vacuum archived systemd journal logs older than the minimum age.",
		DescZH: "使用 journalctl --vacuum-time 清理超过保留天数的归档系统日志。",
	},
	"sys-tmp": {
		NameZH: "系统临时目录 (/tmp, /var/tmp)",
		DescEN: "Remove stale temporary files in /tmp and /var/tmp older than the threshold.",
		DescZH: "清理 /tmp 与 /var/tmp 下超过保留天数的陈旧临时文件。",
	},
	"sys-varcache": {
		NameZH: "/var/cache 通用缓存",
		DescEN: "Clean stale manual pages and fontconfig caches under /var/cache.",
		DescZH: "清理 /var/cache/man 与 /var/cache/fontconfig 下过期的系统缓存。",
	},
	"sys-nix-gc": {
		NameZH:  "Nix 存储垃圾回收",
		DescEN:  "Run native nix-collect-garbage without deleting system or user generations.",
		DescZH:  "调用 nix-collect-garbage 回收未被引用的 Nix 存储对象（不删除历史 generation）。",
		HeavyEN: "Removes unreferenced store paths; rebuilding unlisted derivations may re-download or compile.",
		HeavyZH: "会移除未被引用的 Nix store 路径，之后构建未固定的 derivation 可能需要重新下载或编译。",
	},
	"desktop-flatpak-system": {
		NameZH:  "系统级 Flatpak 未使用运行时",
		DescEN:  "Remove unused system-wide Flatpak runtimes and extensions via flatpak uninstall --unused.",
		DescZH:  "通过 flatpak uninstall --system --unused 清理系统范围内不再被任何应用依赖的运行时。",
		HeavyEN: "Uninstalls unused Flatpak runtimes; installing related apps later will re-download them.",
		HeavyZH: "会卸载未被引用的系统级 Flatpak 运行时，后续安装依赖该运行时的应用需重新下载。",
	},
	"container-docker-builder": {
		NameZH: "Docker 构建缓存 (系统级)",
		DescEN: "Prune unused rootful Docker BuildKit cache entries older than the age filter.",
		DescZH: "清理系统级 Docker 守护进程中过期的 BuildKit 构建缓存。",
	},
	"container-docker-images": {
		NameZH: "Docker 未使用镜像 (系统级)",
		DescEN: "Prune unused rootful Docker images older than the age filter (keeps images in use).",
		DescZH: "清理系统级 Docker 中超过保留天数的未使用镜像（正在使用的镜像不受影响）。",
	},
	"container-docker-containers": {
		NameZH: "Docker 已停止容器 (系统级)",
		DescEN: "Remove stopped rootful Docker containers older than the age filter.",
		DescZH: "清理系统级 Docker 中已停止且超过保留天数的容器。",
	},
	"container-docker-networks": {
		NameZH: "Docker 未使用网络 (系统级)",
		DescEN: "Prune unused rootful Docker networks older than the age filter.",
		DescZH: "清理系统级 Docker 中超过保留天数的未使用网络。",
	},
	"container-podman-system": {
		NameZH: "Podman 系统清理 (不含数据卷)",
		DescEN: "Prune stopped containers, unused images, and build cache via rootful podman system prune (volumes untouched).",
		DescZH: "调用系统级 podman system prune 清理容器、镜像与网络缓存（绝不触碰数据卷）。",
	},

	// User scope: AI & GPU
	"ai-codex-tmp": {
		NameZH: "Codex 临时残留文件",
		DescEN: "Remove interrupted scratch data left by the Codex CLI; sessions are untouched.",
		DescZH: "清理 Codex CLI 中断任务遗留的临时目录，不影响会话记录与配置。",
	},
	"ai-huggingface-incomplete": {
		NameZH: "Hugging Face 未完成下载碎片",
		DescEN: "Remove stale incomplete Hugging Face download artifacts after lock and open-file checks.",
		DescZH: "在确认无文件锁与进程占用后，清理过期的 Hugging Face 未完成下载碎片。",
	},
	"ai-huggingface-datasets-arrow": {
		NameZH:  "Hugging Face Datasets Arrow 缓存",
		DescEN:  "Atomically remove cold, complete Hugging Face Datasets Arrow cache units.",
		DescZH:  "以原子日志方式清理长期未访问的完整 Hugging Face Datasets Arrow 缓存单元。",
		HeavyEN: "Deletes large cached Arrow dataset files; reusing those datasets requires re-downloading or regenerating.",
		HeavyZH: "会删除体积较大的 Arrow 数据集缓存，再次加载对应数据集时需重新下载或生成。",
	},
	"ai-huggingface-prune": {
		NameZH:  "Hugging Face Hub 缓存修剪",
		DescEN:  "Run native hf cache prune on the target Hugging Face cache directory.",
		DescZH:  "调用官方 hf cache prune 修剪 Hugging Face 模型与数据集缓存。",
		HeavyEN: "Delegates to hf cache prune and may remove cached model weights or snapshots.",
		HeavyZH: "将调用 hf cache prune，可能清理已缓存的模型权重或历史快照，再次使用需重新拉取。",
	},
	"ai-torch-hub": {
		NameZH:  "Torch Hub 模型权重缓存",
		DescEN:  "Atomically remove cold Torch Hub checkpoint files via recovery-journaled deletion.",
		DescZH:  "通过带恢复日志的原子删除清理长期未使用的 PyTorch Hub 模型权重。",
		HeavyEN: "Deletes cached Torch Hub model weights; models will be re-downloaded on next load.",
		HeavyZH: "会删除本地缓存的 Torch Hub 权重文件，下次加载模型时需要重新下载。",
	},
	"gpu-nvidia-compute-cache": {
		NameZH: "NVIDIA CUDA 计算缓存",
		DescEN: "Clean old NVIDIA JIT/compute shader cache entries when no GPU job holds them.",
		DescZH: "清理过期的 NVIDIA CUDA 计算编译缓存（若进程或设备占用则自动跳过）。",
	},
	"gpu-triton-cache": {
		NameZH: "Triton 内核编译缓存",
		DescEN: "Clean old Triton compiled kernel cache entries with process guards.",
		DescZH: "清理过期的 Triton 内核编译缓存（包含运行进程保护）。",
	},
	"gpu-torch-inductor-cache": {
		NameZH: "TorchInductor 编译缓存",
		DescEN: "Clean old PyTorch TorchInductor compilation artifacts with process guards.",
		DescZH: "清理过期的 PyTorch TorchInductor 编译缓存产物（包含运行进程保护）。",
	},

	// User scope: Browsers
	"browser-firefox": {
		NameZH: "Firefox 浏览器缓存",
		DescEN: "Clean Firefox cache2, startupCache, and OfflineCache subdirectories; profiles and logins stay intact.",
		DescZH: "清理 Firefox 的 cache2、startupCache 与 OfflineCache 缓存目录，保留配置与登录状态。",
	},
	"browser-chromium": {
		NameZH: "Chromium 系浏览器缓存",
		DescEN: "Clean Cache, Code Cache, and GPUCache for Chrome, Chromium, and Edge; profiles stay intact.",
		DescZH: "清理 Chrome、Chromium、Edge 的 Cache、Code Cache 与 GPUCache，保留书签与登录状态。",
	},
	"browser-puppeteer": {
		NameZH: "Puppeteer 浏览器下载缓存",
		DescEN: "Clean stale Puppeteer-managed Chrome downloads; the age filter keeps recently fetched versions.",
		DescZH: "清理过期未用的 Puppeteer 托管 Chrome 下载缓存，保留近期版本。",
	},

	// User scope: Rootless Containers
	"container-rootless-docker-builder": {
		NameZH: "Docker 构建缓存 (Rootless)",
		DescEN: "Prune unused rootless Docker BuildKit cache after verifying the user-owned rootless socket.",
		DescZH: "校验用户级 Rootless Docker 套接字后，清理过期的 BuildKit 构建缓存。",
	},
	"container-rootless-docker-images": {
		NameZH: "Docker 未使用镜像 (Rootless)",
		DescEN: "Prune unused rootless Docker images older than the age filter (keeps images in use).",
		DescZH: "校验用户级 Rootless Docker 后，清理超过保留天数的未使用镜像。",
	},
	"container-rootless-podman-system": {
		NameZH: "Podman 系统清理 (Rootless)",
		DescEN: "Prune rootless Podman stopped containers, unused images, and build cache (volumes untouched).",
		DescZH: "调用用户级 Rootless Podman 清理停止容器、未使用镜像与构建缓存（不删除数据卷）。",
	},

	// User scope: Desktop
	"desktop-download-partials": {
		NameZH: "Downloads 未完成下载碎片",
		DescEN: "Remove only old, single-link interrupted download sidecars (.part/.crdownload/.tmp); regular downloads are never touched.",
		DescZH: "仅清理 Downloads 目录顶层过期且中断的临时下载后缀文件（如 .part/.crdownload），绝不触碰普通文件。",
	},
	"desktop-trash": {
		NameZH: "桌面回收站",
		DescEN: "Remove old items from the user's freedesktop Trash (~/.local/share/Trash).",
		DescZH: "清理桌面回收站（~/.local/share/Trash）中超过保留天数的旧文件。",
	},
	"desktop-thumbnails": {
		NameZH: "桌面缩略图缓存",
		DescEN: "Clean old generated image and video thumbnails in ~/.cache/thumbnails.",
		DescZH: "清理 ~/.cache/thumbnails 中过期的文件管理器缩略图缓存。",
	},
	"desktop-flatpak-user": {
		NameZH:  "用户级 Flatpak 未使用运行时",
		DescEN:  "Remove unused user-scope Flatpak runtimes via flatpak uninstall --user --unused.",
		DescZH:  "通过 flatpak uninstall --user --unused 清理用户范围内不再被依赖的运行时。",
		HeavyEN: "Uninstalls unused user Flatpak runtimes; future app installs may re-download them.",
		HeavyZH: "会卸载未被引用的用户级 Flatpak 运行时，后续安装关联应用时需重新下载。",
	},

	// User scope: Editors & Shells
	"editor-vscode": {
		NameZH: "VS Code 缓存与过期服务端包",
		DescEN: "Clean VS Code blob/code cache, logs, cpptools cache, and cached Server VSIXs; rebuilt on launch.",
		DescZH: "清理 VS Code 缓存、日志、cpptools 缓存及服务端 VSIX 安装包（重启后自动重建）。",
	},
	"editor-jetbrains": {
		NameZH: "JetBrains IDE 缓存",
		DescEN: "Clean old JetBrains IDE index and system caches under ~/.cache/JetBrains; skips if an IDE is open.",
		DescZH: "清理 ~/.cache/JetBrains 下的索引与系统缓存（检测到 IDE 运行时自动跳过）。",
	},
	"editor-nvim": {
		NameZH: "Neovim 状态日志与交换文件",
		DescEN: "Clean old Neovim log, swap, and undo state files under ~/.local/state/nvim.",
		DescZH: "清理 ~/.local/state/nvim 下过期的 log、swap 与 undo 状态文件。",
	},
	"shell-zsh": {
		NameZH: "Zsh 补全缓存",
		DescEN: "Clean old zsh cache files and stale .zcompdump files.",
		DescZH: "清理过期的 zsh 缓存目录及旧的 .zcompdump 补全转储文件。",
	},

	// User scope: Kubernetes & Cloud
	"k8s-helm-cache": {
		NameZH: "Helm 仓库与 Chart 缓存",
		DescEN: "Clean old Helm repository index and chart archives under ~/.cache/helm.",
		DescZH: "清理 ~/.cache/helm 下过期的 Chart 与仓库索引缓存。",
	},
	"k8s-k9s-cache": {
		NameZH: "k9s 缓存",
		DescEN: "Clean old k9s temporary state and screen caches under ~/.cache/k9s.",
		DescZH: "清理 ~/.cache/k9s 下过期的临时缓存数据。",
	},
	"k8s-kubectl-cache": {
		NameZH: "kubectl 发现缓存",
		DescEN: "Clean ~/.kube/cache and ~/.kube/http-cache only; ~/.kube/config is strictly protected.",
		DescZH: "仅清理 ~/.kube/cache 与 ~/.kube/http-cache，绝不触碰 ~/.kube/config 凭据。",
	},
	"k8s-minikube-cache": {
		NameZH: "minikube 镜像与 ISO 缓存",
		DescEN: "Clean old cached images and ISOs under ~/.minikube/cache.",
		DescZH: "清理 ~/.minikube/cache 中过期的镜像与 ISO 缓存。",
	},
	"cloud-gcloud-logs": {
		NameZH: "Google Cloud CLI 日志",
		DescEN: "Clean old gcloud CLI log files at the fixed target-home config log root.",
		DescZH: "清理用户目录下过期陈旧的 gcloud CLI 运行日志。",
	},
	"cloud-azure-logs": {
		NameZH: "Azure CLI 日志",
		DescEN: "Clean old Azure CLI log files at the fixed target-home config log root.",
		DescZH: "清理用户目录下过期陈旧的 Azure CLI 运行日志。",
	},

	// User scope: Languages & Dev Tools
	"lang-bun": {
		NameZH: "Bun 安装缓存",
		DescEN: "Clean old packages from Bun's install cache directory.",
		DescZH: "清理 Bun 包管理器安装缓存中过期的依赖文件。",
	},
	"lang-cargo": {
		NameZH: "Cargo .crate 下载缓存",
		DescEN: "Clean downloaded .crate archives in Cargo registry cache; packages are re-downloaded on demand.",
		DescZH: "清理 Cargo 下载的 .crate 压缩包缓存，需要时可按需重新下载。",
	},
	"lang-go": {
		NameZH: "Go 编译缓存",
		DescEN: "Clean stale build artifacts inside Go's GOCACHE directory.",
		DescZH: "清理 Go GOCACHE 目录中超过保留天数的构建缓存产物。",
	},
	"lang-gradle": {
		NameZH: "Gradle 缓存",
		DescEN: "Clean old Gradle cache artifacts under $GRADLE_USER_HOME/caches.",
		DescZH: "清理 $GRADLE_USER_HOME/caches 中过期的 Gradle 构建与依赖缓存。",
	},
	"lang-maven": {
		NameZH: "Maven 下载缓存",
		DescEN: "Clean old metadata under Maven local repository .cache.",
		DescZH: "清理 Maven 本地仓库 .cache 目录中过期的下载缓存（保留正式构件）。",
	},
	"lang-npm": {
		NameZH: "npm 缓存",
		DescEN: "Clean stale npm _cacache, _npx, _logs, and _prebuilds directories.",
		DescZH: "清理 npm 缓存目录下的 _cacache、_npx、_logs 与 _prebuilds 过期文件。",
	},
	"lang-pip": {
		NameZH: "pip Wheel 缓存",
		DescEN: "Clean stale downloaded wheels and HTTP cache in pip's cache directory.",
		DescZH: "清理 pip 缓存目录中过期的 wheel 安装包与 HTTP 缓存。",
	},
	"lang-pixi": {
		NameZH: "Pixi / Rattler 缓存",
		DescEN: "Clean stale Rattler package and repodata cache shared by pixi and rattler-based tools.",
		DescZH: "清理 pixi 与 rattler 工具共用的包缓存与索引缓存。",
	},
	"lang-pnpm": {
		NameZH: "pnpm Store 垃圾回收",
		DescEN: "Run native pnpm store prune to remove unreferenced packages from the content-addressable store.",
		DescZH: "调用官方 pnpm store prune 清理内容寻址仓库中不再被引用的依赖包。",
	},
	"lang-yarn": {
		NameZH: "Yarn 缓存",
		DescEN: "Clean stale Yarn package cache archives.",
		DescZH: "清理 Yarn 缓存目录中过期的软件包压缩归档。",
	},
	"lang-uv": {
		NameZH: "uv 缓存修剪",
		DescEN: "Run native uv cache prune to garbage-collect unreferenced Python wheel/source cache.",
		DescZH: "调用官方 uv cache prune 回收不再被引用的 Python 包缓存。",
	},
	"lang-conda": {
		NameZH: "Conda 索引与压缩包缓存",
		DescEN: "Run native conda clean limited to index cache, tarballs, and logfiles (environments untouched).",
		DescZH: "调用 conda clean 清理索引缓存、tarball 安装包与日志（不删除任何虚拟环境）。",
	},
	"lang-rustup": {
		NameZH:  "Rust 过期工具链",
		DescEN:  "Remove stale per-release-line Rust toolchains while keeping default, newest in each line, and linked toolchains.",
		DescZH:  "清理旧版本 Rust 工具链（保留默认工具链、各分支最新版本及项目链接工具链）。",
		HeavyEN: "Uninstalls older Rust toolchains; projects pinned via rust-toolchain.toml to an old version will re-download it.",
		HeavyZH: "会卸载旧版 Rust 工具链；若有项目锁定旧版本编译，下次使用时需重新安装对应工具链。",
	},
	"lang-corepack": {
		NameZH:  "Corepack 包管理器缓存",
		DescEN:  "Run native corepack cache clean to remove cached package-manager binaries.",
		DescZH:  "调用 corepack cache clean 清理缓存的包管理器二进制文件。",
		HeavyEN: "Clears all cached Corepack package manager shims; offline invocation will require re-downloading.",
		HeavyZH: "会清除所有已缓存的 Corepack 包管理器版本，下次离线切换版本需联网重新下载。",
	},
	"lang-poetry-cache": {
		NameZH:  "Poetry PyPI 缓存",
		DescEN:  "Run native poetry cache clear pypi --all to purge Poetry's PyPI artifact cache.",
		DescZH:  "调用 poetry cache clear pypi --all 清理 Poetry 的 PyPI 下载缓存。",
		HeavyEN: "Removes all cached PyPI downloads inside Poetry; reinstalling dependencies will fetch them again.",
		HeavyZH: "会清空 Poetry 的全部 PyPI 下载缓存，重新安装依赖时需再次联网拉取。",
	},
	"pkg-homebrew": {
		NameZH: "Homebrew 下载与配方缓存",
		DescEN: "Run native brew cleanup --prune to remove outdated downloads and formula cache.",
		DescZH: "调用 brew cleanup --prune 清理过期的 Homebrew 下载包与配方缓存。",
	},
	"dev-pre-commit": {
		NameZH: "pre-commit 仓库垃圾回收",
		DescEN: "Run native pre-commit gc to clean unused hook environments.",
		DescZH: "调用 pre-commit gc 清理不再使用的钩子仓库缓存环境。",
	},
	"dev-mise": {
		NameZH:  "mise 工具与下载缓存",
		DescEN:  "Run native mise prune --yes to clean unused tool versions and download caches.",
		DescZH:  "调用 mise prune --yes 清理未被引用的工具版本与下载缓存。",
		HeavyEN: "Removes unused tool versions managed by mise; projects entering pinned directories may re-fetch them.",
		HeavyZH: "会删除未被当前配置引用的 mise 工具版本，进入特定目录时可能需重新下载对应版本。",
	},
	"test-cypress": {
		NameZH: "Cypress 二进制缓存",
		DescEN: "Run native cypress cache prune to remove unused Cypress test runner versions.",
		DescZH: "调用 cypress cache prune 清理不再使用的 Cypress 测试浏览器版本。",
	},
	"test-playwright-browsers": {
		NameZH:  "Playwright 浏览器二进制",
		DescEN:  "Run native playwright uninstall --all through the installed Playwright binary.",
		DescZH:  "调用 playwright uninstall --all 卸载已下载的 Playwright 浏览器内核。",
		HeavyEN: "Removes ALL installed Playwright browser binaries; running browser tests afterward requires npx playwright install.",
		HeavyZH: "会卸载全部 Playwright 浏览器内核，后续跑端到端测试前需重新执行 playwright install。",
	},
	"build-ccache": {
		NameZH: "ccache 编译缓存整理",
		DescEN: "Run native ccache --cleanup to purge stale compiler cache entries.",
		DescZH: "调用 ccache --cleanup 整理并清理过期的 C/C++ 编译缓存条目。",
	},
	"python-ruff-cache": {
		NameZH: "Ruff 静态检查缓存",
		DescEN: "Clean only an explicit RUFF_CACHE_DIR beneath target home.",
		DescZH: "仅清理用户目录下由环境变量 RUFF_CACHE_DIR 显式指定的 Ruff 缓存。",
	},
	"python-mypy-cache": {
		NameZH: "mypy 类型检查缓存",
		DescEN: "Clean only an explicit MYPY_CACHE_DIR beneath target home.",
		DescZH: "仅清理用户目录下由环境变量 MYPY_CACHE_DIR 显式指定的 mypy 缓存。",
	},
	"python-pyinstaller-cache": {
		NameZH: "PyInstaller 打包缓存",
		DescEN: "Clean PyInstaller's fixed XDG cache root (~/.cache/pyinstaller).",
		DescZH: "清理 PyInstaller 固定缓存目录（~/.cache/pyinstaller）中的过期产物。",
	},
}

// CleanerName returns the localized display name for cleaner c.
// CleanerName 返回清理器 c 的本地化显示名称。
func (t *Translator) CleanerName(c model.Cleaner) string {
	if t.IsZH() {
		if loc, ok := builtinCleanerLocales[c.ID]; ok && loc.NameZH != "" {
			return loc.NameZH
		}
	}
	if c.Name != "" {
		return c.Name
	}
	return c.ID
}

// CleanerDesc returns the localized description for cleaner c, falling back across
// YAML description and built-in descriptions.
// CleanerDesc 返回清理器 c 的本地化描述，必要时在 YAML 描述与内置文案间回退。
func (t *Translator) CleanerDesc(c model.Cleaner) string {
	loc, hasLoc := builtinCleanerLocales[c.ID]
	if t.IsZH() {
		if hasLoc && loc.DescZH != "" {
			return loc.DescZH
		}
		if c.Description != "" {
			return c.Description
		}
		if hasLoc {
			return loc.DescEN
		}
		return ""
	}
	if c.Description != "" {
		return c.Description
	}
	if hasLoc && loc.DescEN != "" {
		return loc.DescEN
	}
	return ""
}

// HeavyNote returns a localized warning explaining why a heavy cleaner requires opt-in.
// HeavyNote 返回解释为何重度清理项需要手动授权的本地化风险提示。
func (t *Translator) HeavyNote(c model.Cleaner) string {
	if c.EffectiveImpact() != model.ImpactHeavy {
		return ""
	}
	if loc, ok := builtinCleanerLocales[c.ID]; ok {
		if t.IsZH() && loc.HeavyZH != "" {
			return loc.HeavyZH
		}
		if !t.IsZH() && loc.HeavyEN != "" {
			return loc.HeavyEN
		}
	}
	return t.Text(
		"Heavy cleaner: removes large reusable artifacts or tool versions; explicit opt-in required.",
		"重度清理项：可能删除体积较大的可重建产物或旧版工具链，需显式授权才会执行。",
	)
}
