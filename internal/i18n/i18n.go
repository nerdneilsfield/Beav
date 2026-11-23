// Package i18n provides bilingual (English and Chinese) UI translations and locale detection.
// Package i18n 提供中英文双语界面翻译与区域设置检测。
package i18n

import (
	"fmt"
	"os"
	"strings"

	"github.com/dengqi/beav/internal/cleaner/model"
)

// Lang identifies the UI display language.
// Lang 标识界面显示语言。
type Lang string

const (
	// LangEN selects English UI strings.
	// LangEN 选择英文界面文案。
	LangEN Lang = "en"
	// LangZH selects Simplified Chinese UI strings.
	// LangZH 选择简体中文界面文案。
	LangZH Lang = "zh"
)

// ParseLang parses a user-supplied language flag or locale string.
// Empty or "auto" returns ("", true) to indicate environment detection.
// ParseLang 解析用户指定的语言参数或区域设置字符串。
// 空值或 "auto" 返回 ("", true)，表示按环境变量自动检测。
func ParseLang(raw string) (Lang, bool) {
	v := strings.ToLower(strings.TrimSpace(raw))
	if v == "" || v == "auto" {
		return "", true
	}
	v, _, _ = strings.Cut(v, ".")
	v, _, _ = strings.Cut(v, "@")
	if v == "en" || strings.HasPrefix(v, "en_") || strings.HasPrefix(v, "en-") {
		return LangEN, true
	}
	if v == "zh" || strings.HasPrefix(v, "zh_") || strings.HasPrefix(v, "zh-") {
		return LangZH, true
	}
	return "", false
}

// DetectFrom determines the UI language from an explicit flag value and environment getter.
// DetectFrom 根据显式语言标志与环境变量读取函数确定界面语言。
func DetectFrom(flagVal string, getenv func(string) string) Lang {
	if l, ok := ParseLang(flagVal); ok && l != "" {
		return l
	}
	if getenv == nil {
		return LangEN
	}
	for _, key := range []string{"BEAV_LANG", "LC_ALL", "LC_MESSAGES", "LANG"} {
		raw := strings.TrimSpace(getenv(key))
		if raw == "" {
			continue
		}
		if strings.EqualFold(raw, "C") || strings.EqualFold(raw, "POSIX") {
			return LangEN
		}
		if l, ok := ParseLang(raw); ok && l != "" {
			return l
		}
		// Any recognized non-auto locale that isn't zh falls back to English.
		return LangEN
	}
	return LangEN
}

// DetectFromArgs pre-scans CLI arguments for --lang before Cobra parses flags so
// help output also honors the requested language.
// DetectFromArgs 在 Cobra 解析标志前预扫描 --lang 参数，使帮助文本也能按指定语言输出。
func DetectFromArgs(args []string, getenv func(string) string) Lang {
	flagVal := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			break
		}
		if strings.HasPrefix(arg, "--lang=") {
			flagVal = strings.TrimPrefix(arg, "--lang=")
			continue
		}
		if arg == "--lang" && i+1 < len(args) {
			flagVal = args[i+1]
			i++
		}
	}
	return DetectFrom(flagVal, getenv)
}

// Detect determines the UI language using the current process environment.
// Detect 使用当前进程环境变量确定界面语言。
func Detect(flagVal string) Lang {
	return DetectFrom(flagVal, os.Getenv)
}

// Translator formats bilingual strings for a chosen UI language.
// Translator 根据选定界面语言格式化双语文案。
type Translator struct {
	lang Lang
}

// New creates a Translator for the specified language, defaulting unknown values to English.
// New 为指定语言创建 Translator，未知值默认回退为英文。
func New(lang Lang) *Translator {
	if lang != LangZH {
		lang = LangEN
	}
	return &Translator{lang: lang}
}

// Lang returns the active language code.
// Lang 返回当前激活的语言代码。
func (t *Translator) Lang() Lang {
	if t == nil || t.lang != LangZH {
		return LangEN
	}
	return LangZH
}

// IsZH reports whether the active language is Chinese.
// IsZH 返回当前语言是否为中文。
func (t *Translator) IsZH() bool {
	return t.Lang() == LangZH
}

// Text returns en or zh according to the active language.
// Text 根据当前语言返回英文或中文文本。
func (t *Translator) Text(en, zh string) string {
	if t.IsZH() {
		return zh
	}
	return en
}

// Textf formats en or zh with fmt.Sprintf according to the active language.
// Textf 根据当前语言使用 fmt.Sprintf 格式化英文或中文文本。
func (t *Translator) Textf(en, zh string, args ...any) string {
	return fmt.Sprintf(t.Text(en, zh), args...)
}

// Plural formats a count with English singular/plural forms or a Chinese counter unit.
// Plural 根据当前语言格式化数量（英文区分单复数，中文使用指定量词/名词）。
func (t *Translator) Plural(n int64, singularEN, pluralEN, zhUnit string) string {
	if t.IsZH() {
		return fmt.Sprintf("%d %s", n, zhUnit)
	}
	if n == 1 {
		return fmt.Sprintf("1 %s", singularEN)
	}
	return fmt.Sprintf("%d %s", n, pluralEN)
}

// Days formats a day count in compact EN or ZH style.
// Days 以紧凑的中英文格式显示天数。
func (t *Translator) Days(n int) string {
	if t.IsZH() {
		return fmt.Sprintf("%d 天", n)
	}
	return fmt.Sprintf("%dd", n)
}

// ScopeLabel returns a localized label for a cleaner or run scope.
// ScopeLabel 返回作用域的本地化标签。
func (t *Translator) ScopeLabel(s model.Scope) string {
	switch s {
	case model.ScopeUser:
		return t.Text("user", "用户")
	case model.ScopeSystem:
		return t.Text("system", "系统")
	case model.ScopeAll:
		return t.Text("user + system", "用户 + 系统")
	default:
		return string(s)
	}
}

// ImpactLabel returns a localized label for a cleaner impact level.
// ImpactLabel 返回清理器影响级别的本地化标签。
func (t *Translator) ImpactLabel(imp model.Impact) string {
	if imp == model.ImpactHeavy {
		return t.Text("heavy", "重度")
	}
	return t.Text("safe", "安全")
}

// SkipPhrase maps a machine skip reason to a concise localized explanation.
// SkipPhrase 将机器跳过原因映射为简明的本地化说明。
func (t *Translator) SkipPhrase(reason string) string {
	switch reason {
	case "root_unavailable":
		return t.Text("cache directory not present", "缓存目录不存在或未配置")
	case string(model.SkipReasonNoMatches):
		return t.Text("no matching files", "未发现匹配缓存")
	case string(model.SkipReasonHeavyNotAuthorized):
		return t.Text("heavy cleaner not authorized", "重度清理项未授权")
	case string(model.SkipReasonRunningProcess):
		return t.Text("application is running", "对应应用正在运行")
	case "cache_busy":
		return t.Text("cache is busy or locked", "缓存正被占用或已加锁")
	case string(model.SkipReasonRuntimeUnavailable):
		return t.Text("runtime unavailable", "容器运行时不可用")
	case string(model.SkipReasonRuntimeNotRootless):
		return t.Text("daemon is not rootless", "守护进程非 rootless 模式")
	case string(model.SkipReasonRuntimeBusy):
		return t.Text("containers are running", "有容器正在运行")
	case string(model.SkipReasonManagerNotInstalled):
		return t.Text("tool not installed", "未安装对应工具")
	case string(model.SkipReasonDryRunUnsupported):
		return t.Text("native command (no dry-run preview)", "原生命令（不支持预演估算）")
	case string(model.SkipReasonAgeTooRecent):
		return t.Text("files newer than min age", "文件未超过保留天数")
	case string(model.SkipReasonWhitelisted):
		return t.Text("protected by whitelist", "已被白名单保护")
	case string(model.SkipReasonBlacklisted):
		return t.Text("protected system/personal path", "受保护路径已拦截")
	case string(model.SkipReasonKeepListIntersection):
		return t.Text("intersects protected keep list", "与受保护保留列表冲突")
	case string(model.SkipReasonExcluded):
		return t.Text("excluded by rule", "被排除规则跳过")
	case string(model.SkipReasonBoundaryViolation):
		return t.Text("outside allowed safety boundary", "超出安全路径边界")
	case string(model.SkipReasonSymlink):
		return t.Text("symlink skipped for safety", "为安全起见跳过符号链接")
	case string(model.SkipReasonCrossFS):
		return t.Text("cross-filesystem boundary skipped", "跳过跨文件系统路径")
	case string(model.SkipReasonPermissionDenied):
		return t.Text("permission denied", "权限不足")
	case "capability_unsupported":
		return t.Text("installed tool lacks required flag", "当前工具版本缺少所需参数")
	case "version_mismatch":
		return t.Text("unexpected tool binary/version", "工具版本或二进制校验不匹配")
	case "safety_changed":
		return t.Text("target state changed during check", "检查期间目标状态发生变化")
	case "identity_unavailable":
		return t.Text("target user identity unavailable", "无法确认目标用户身份")
	case "profile_scope_mismatch":
		return t.Text("cleaner scope does not match profile", "清理器作用域与配置不匹配")
	default:
		return strings.ReplaceAll(reason, "_", " ")
	}
}

// SkipRemedy returns an actionable suggestion for a skip reason, or "" if no user action is needed.
// SkipRemedy 返回针对跳过原因的可执行建议；若属于正常环境跳过则返回空字符串。
func (t *Translator) SkipRemedy(reason string) string {
	switch reason {
	case string(model.SkipReasonHeavyNotAuthorized):
		return t.Text("pass --include-heavy or --only <id>", "加 --include-heavy 或 --only <ID>")
	case string(model.SkipReasonRunningProcess):
		return t.Text("close the app and retry", "关闭对应应用后重试")
	case "cache_busy", string(model.SkipReasonRuntimeBusy):
		return t.Text("wait for active tasks to finish and retry", "等待当前任务完成后重试")
	case string(model.SkipReasonRuntimeNotRootless):
		return t.Text("use sudo beav clean --system for rootful daemon", "系统守护进程请运行 sudo beav clean --system")
	case string(model.SkipReasonAgeTooRecent):
		return t.Text("lower --min-age to include newer files", "可调小 --min-age 包含较新文件")
	default:
		return ""
	}
}

// IsQuietSkip reports whether a skip reason is normal environmental absence (uninstalled tool or missing cache)
// that can be collapsed in concise output.
// IsQuietSkip 判断跳过原因是否属于正常的环境缺失（工具未装或缓存目录不存在），可在简洁模式下折叠汇总。
func IsQuietSkip(reason string) bool {
	switch reason {
	case string(model.SkipReasonNoMatches),
		"root_unavailable",
		string(model.SkipReasonManagerNotInstalled),
		string(model.SkipReasonRuntimeUnavailable):
		return true
	default:
		return false
	}
}
