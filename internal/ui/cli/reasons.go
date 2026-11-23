package cli

import "github.com/dengqi/beav/internal/cleaner/model"

// skipPhrase maps a machine skip reason to a short human phrase.
// skipPhrase 将机器跳过原因映射为简短的人类可读短语。
func skipPhrase(reason string) string {
	switch model.SkipReason(reason) {
	case "root_unavailable":
		return "root path unavailable"
	case model.SkipReasonNoMatches:
		return "no matching files"
	case model.SkipReasonHeavyNotAuthorized:
		return "heavy, not authorized"
	case model.SkipReasonRunningProcess:
		return "tool is running"
	case model.SkipReasonRuntimeUnavailable:
		return "runtime not available"
	case model.SkipReasonRuntimeNotRootless:
		return "daemon is not rootless"
	case model.SkipReasonRuntimeBusy:
		return "runtime busy"
	case model.SkipReasonManagerNotInstalled:
		return "manager not installed"
	case model.SkipReasonDryRunUnsupported:
		return "native command, no preview"
	case model.SkipReasonAgeTooRecent:
		return "files too recent"
	case model.SkipReasonWhitelisted:
		return "path is whitelisted"
	case model.SkipReasonBlacklisted:
		return "path is protected"
	case model.SkipReasonKeepListIntersection:
		return "path is on the keep list"
	case model.SkipReasonExcluded:
		return "excluded"
	default:
		return reason
	}
}

// skipRemedy returns the action a user can take to make the skip go away, or
// "" when no user action exists.
// skipRemedy 返回用户可以采取的补救动作；无法补救时返回空字符串。
func skipRemedy(reason string) string {
	switch model.SkipReason(reason) {
	case model.SkipReasonHeavyNotAuthorized:
		return "pass --include-heavy or put the exact ID in --only"
	case "root_unavailable":
		return "run with sudo if you want this cleaner"
	case model.SkipReasonRunningProcess:
		return "close the tool and rerun"
	case model.SkipReasonAgeTooRecent:
		return "wait, or lower --min-age"
	case model.SkipReasonManagerNotInstalled, model.SkipReasonRuntimeUnavailable:
		return "install the tool if you want this cleaner"
	default:
		return ""
	}
}
