package model

import "time"

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
