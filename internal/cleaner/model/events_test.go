package model

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestEventJSONShapesByEventType(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name string
		ev   Event
		want []string
		drop []string
	}{
		{
			name: "start includes dry_run false",
			ev:   Event{Event: EvStart, CleanerID: "c", Name: "C", Scope: ScopeUser, Type: TypePaths, DryRun: false, TS: now},
			want: []string{`"dry_run":false`, `"scope":"user"`, `"type":"paths"`},
		},
		{
			name: "skipped omits counters",
			ev:   Event{Event: EvSkipped, CleanerID: "c", Path: "/tmp/x", Reason: "excluded", TS: now},
			want: []string{`"reason":"excluded"`},
			drop: []string{`"files_deleted":`, `"bytes_freed":`, `"dry_run":`},
		},
		{
			name: "finish keeps zero counters",
			ev:   Event{Event: EvFinish, CleanerID: "c", Status: "ok", TS: now},
			want: []string{`"files_deleted":0`, `"bytes_freed":0`, `"errors":0`, `"duration_ms":0`},
		},
		{
			name: "summary keeps zero counters",
			ev:   Event{Event: EvSummary, TS: now},
			want: []string{`"cleaners_run":0`, `"cleaners_skipped":0`, `"cleaners_errored":0`},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(tc.ev)
			if err != nil {
				t.Fatal(err)
			}
			s := string(b)
			for _, want := range tc.want {
				if !strings.Contains(s, want) {
					t.Fatalf("json %s missing %s", s, want)
				}
			}
			for _, drop := range tc.drop {
				if strings.Contains(s, drop) {
					t.Fatalf("json %s unexpectedly contains %s", s, drop)
				}
			}
		})
	}
}
