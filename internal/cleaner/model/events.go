package model

import (
	"encoding/json"
	"time"
)

type EventType string

const (
	EvStart          EventType = "start"
	EvDeleted        EventType = "deleted"
	EvSkipped        EventType = "skipped"
	EvCleanerSkipped EventType = "cleaner_skipped"
	EvCommandOutput  EventType = "command_output"
	EvError          EventType = "error"
	EvFinish         EventType = "finish"
	EvSummary        EventType = "summary"
)

type Event struct {
	Event           EventType    `json:"event"`
	CleanerID       string       `json:"cleaner_id,omitempty"`
	Name            string       `json:"name,omitempty"`
	Scope           Scope        `json:"scope,omitempty"`
	Type            ExecutorType `json:"type,omitempty"`
	DryRun          bool         `json:"dry_run"`
	Path            string       `json:"path,omitempty"`
	Size            int64        `json:"size"`
	Reason          string       `json:"reason,omitempty"`
	Detail          string       `json:"detail,omitempty"`
	Stream          string       `json:"stream,omitempty"`
	Line            string       `json:"line,omitempty"`
	Status          string       `json:"status,omitempty"`
	FilesDeleted    int64        `json:"files_deleted"`
	BytesFreed      int64        `json:"bytes_freed"`
	Errors          int          `json:"errors"`
	DurationMs      int64        `json:"duration_ms"`
	CleanersRun     int          `json:"cleaners_run"`
	CleanersSkipped int          `json:"cleaners_skipped"`
	CleanersErrored int          `json:"cleaners_errored"`
	TS              time.Time    `json:"ts"`
}

func (e Event) MarshalJSON() ([]byte, error) {
	type eventJSON struct {
		Event           EventType    `json:"event"`
		CleanerID       string       `json:"cleaner_id,omitempty"`
		Name            string       `json:"name,omitempty"`
		Scope           Scope        `json:"scope,omitempty"`
		Type            ExecutorType `json:"type,omitempty"`
		DryRun          *bool        `json:"dry_run,omitempty"`
		Path            string       `json:"path,omitempty"`
		Size            *int64       `json:"size,omitempty"`
		Reason          string       `json:"reason,omitempty"`
		Detail          string       `json:"detail,omitempty"`
		Stream          string       `json:"stream,omitempty"`
		Line            string       `json:"line,omitempty"`
		Status          string       `json:"status,omitempty"`
		FilesDeleted    *int64       `json:"files_deleted,omitempty"`
		BytesFreed      *int64       `json:"bytes_freed,omitempty"`
		Errors          *int         `json:"errors,omitempty"`
		DurationMs      *int64       `json:"duration_ms,omitempty"`
		CleanersRun     *int         `json:"cleaners_run,omitempty"`
		CleanersSkipped *int         `json:"cleaners_skipped,omitempty"`
		CleanersErrored *int         `json:"cleaners_errored,omitempty"`
		TS              time.Time    `json:"ts"`
	}
	out := eventJSON{
		Event:     e.Event,
		CleanerID: e.CleanerID,
		Path:      e.Path,
		Reason:    e.Reason,
		Detail:    e.Detail,
		Stream:    e.Stream,
		Line:      e.Line,
		Status:    e.Status,
		TS:        e.TS,
	}
	switch e.Event {
	case EvStart:
		out.Name = e.Name
		out.Scope = e.Scope
		out.Type = e.Type
		out.DryRun = boolPtr(e.DryRun)
	case EvDeleted:
		out.Size = int64Ptr(e.Size)
	case EvSkipped:
		if e.Size != 0 {
			out.Size = int64Ptr(e.Size)
		}
	case EvFinish:
		if e.DryRun {
			out.DryRun = boolPtr(e.DryRun)
		}
		out.FilesDeleted = int64Ptr(e.FilesDeleted)
		out.BytesFreed = int64Ptr(e.BytesFreed)
		out.Errors = intPtr(e.Errors)
		out.DurationMs = int64Ptr(e.DurationMs)
	case EvSummary:
		if e.DryRun {
			out.DryRun = boolPtr(e.DryRun)
		}
		out.FilesDeleted = int64Ptr(e.FilesDeleted)
		out.BytesFreed = int64Ptr(e.BytesFreed)
		out.Errors = intPtr(e.Errors)
		out.DurationMs = int64Ptr(e.DurationMs)
		out.CleanersRun = intPtr(e.CleanersRun)
		out.CleanersSkipped = intPtr(e.CleanersSkipped)
		out.CleanersErrored = intPtr(e.CleanersErrored)
	}
	return json.Marshal(out)
}

func boolPtr(v bool) *bool    { return &v }
func intPtr(v int) *int       { return &v }
func int64Ptr(v int64) *int64 { return &v }
