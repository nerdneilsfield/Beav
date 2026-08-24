package model

// SkipReason is a stable machine-readable reason for a skipped cleaner or entry.
// SkipReason 是清理器或条目跳过原因的稳定机器可读类型。
type SkipReason string

// String returns the stable wire representation.
// String 返回稳定的线协议表示。
func (r SkipReason) String() string { return string(r) }

const (
	SkipReasonRunningProcess       SkipReason = "running_process"
	SkipReasonBoundaryViolation    SkipReason = "boundary_violation"
	SkipReasonNoMatches            SkipReason = "no_matches"
	SkipReasonWhitelisted          SkipReason = "whitelisted"
	SkipReasonExcluded             SkipReason = "excluded"
	SkipReasonDryRun               SkipReason = "dry_run"
	SkipReasonToctouChanged        SkipReason = "toctou_changed"
	SkipReasonAgeTooRecent         SkipReason = "age_too_recent"
	SkipReasonBlacklisted          SkipReason = "blacklisted"
	SkipReasonPermissionDenied     SkipReason = "permission_denied"
	SkipReasonSymlink              SkipReason = "symlink"
	SkipReasonCrossFS              SkipReason = "cross_fs"
	SkipReasonWrongType            SkipReason = "wrong_type"
	SkipReasonManagerNotInstalled  SkipReason = "manager_not_installed"
	SkipReasonRuntimeUnavailable   SkipReason = "runtime_unavailable"
	SkipReasonRuntimeNotRootless   SkipReason = "runtime_not_rootless"
	SkipReasonRuntimeBusy          SkipReason = "runtime_busy"
	SkipReasonHeavyNotAuthorized   SkipReason = "heavy_not_authorized"
	SkipReasonDryRunUnsupported    SkipReason = "dry_run_unsupported"
	SkipReasonMetricsUnmeasured    SkipReason = "metrics_unmeasured"
	SkipReasonKeepListIntersection SkipReason = "keep_list_intersection"
)

// Short aliases keep call sites concise while retaining typed values.
const (
	HeavyNotAuthorized      = SkipReasonHeavyNotAuthorized
	DryRunUnsupported       = SkipReasonDryRunUnsupported
	SkipReasonTOCTOUChanged = SkipReasonToctouChanged
	KeepListIntersection    = SkipReasonKeepListIntersection
)
