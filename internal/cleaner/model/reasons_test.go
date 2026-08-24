package model

import "testing"

func TestSkipReasonValuesRemainStable(t *testing.T) {
	cases := map[SkipReason]string{
		SkipReasonRunningProcess:     "running_process",
		SkipReasonBoundaryViolation:  "boundary_violation",
		SkipReasonNoMatches:          "no_matches",
		SkipReasonWhitelisted:        "whitelisted",
		SkipReasonExcluded:           "excluded",
		SkipReasonDryRun:             "dry_run",
		SkipReasonHeavyNotAuthorized: "heavy_not_authorized",
	}
	for reason, want := range cases {
		if string(reason) != want {
			t.Errorf("reason %q = %q, want %q", reason, reason, want)
		}
	}
}
