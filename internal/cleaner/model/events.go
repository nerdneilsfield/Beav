package model

import (
	"encoding/json"
	"time"
)

// EventType represents the type of event in the cleaning process.
// EventType 表示清理过程中的事件类型。
type EventType string

// EvStart is the event emitted when a cleaner starts.
// EvStart 是清理器启动时发出的事件。
const EvStart EventType = "start"

// EvDeleted is the event emitted when a file is deleted.
// EvDeleted 是文件被删除时发出的事件。
const EvDeleted EventType = "deleted"

// EvSkipped is the event emitted when a file is skipped.
// EvSkipped 是文件被跳过时发出的事件。
const EvSkipped EventType = "skipped"

// EvCleanerSkipped is the event emitted when a cleaner is skipped.
// EvCleanerSkipped 是清理器被跳过时发出的事件。
const EvCleanerSkipped EventType = "cleaner_skipped"

// EvCommandOutput is the event emitted when a command produces output.
// EvCommandOutput 是命令产生输出时发出的事件。
const EvCommandOutput EventType = "command_output"

// EvError is the event emitted when an error occurs.
// EvError 是发生错误时发出的事件。
const EvError EventType = "error"

// EvFinish is the event emitted when a cleaner finishes.
// EvFinish 是清理器完成时发出的事件。
const EvFinish EventType = "finish"

// EvSummary is the event emitted for the overall summary.
// EvSummary 是总体摘要时发出的事件。
const EvSummary EventType = "summary"

// Event represents a cleaning event with detailed information.
// Event 表示一个包含详细信息的清理事件。
type Event struct {
	// Event is the type of the event.
	// Event 是事件的类型。
	Event           EventType    `json:"event"`
	// CleanerID is the ID of the cleaner that generated this event.
	// CleanerID 是生成此事件的清理器 ID。
	CleanerID       string       `json:"cleaner_id,omitempty"`
	// Name is the name of the cleaner.
	// Name 是清理器的名称。
	Name            string       `json:"name,omitempty"`
	// Scope is the scope of the cleaner.
	// Scope 是清理器的作用范围。
	Scope           Scope        `json:"scope,omitempty"`
	// Type is the executor type of the cleaner.
	// Type 是清理器的执行器类型。
	Type            ExecutorType `json:"type,omitempty"`
	// DryRun indicates whether the cleaner is running in dry-run mode.
	// DryRun 表示清理器是否以试运行模式运行。
	DryRun          bool         `json:"dry_run"`
	// Path is the file path involved in the event.
	// Path 是事件中涉及的文件路径。
	Path            string       `json:"path,omitempty"`
	// Size is the file size in bytes.
	// Size 是文件大小（字节）。
	Size            int64        `json:"size"`
	// Reason is the reason for skipping or other actions.
	// Reason 是跳过或其他操作的原因。
	Reason          string       `json:"reason,omitempty"`
	// Detail is additional detail about the event.
	// Detail 是关于事件的附加详细信息。
	Detail          string       `json:"detail,omitempty"`
	// Stream is the output stream (stdout/stderr).
	// Stream 是输出流（stdout/stderr）。
	Stream          string       `json:"stream,omitempty"`
	// Line is a line of output.
	// Line 是一行输出。
	Line            string       `json:"line,omitempty"`
	// Status is the status of the cleaner.
	// Status 是清理器的状态。
	Status          string       `json:"status,omitempty"`
	// FilesDeleted is the number of files deleted.
	// FilesDeleted 是已删除的文件数。
	FilesDeleted    int64        `json:"files_deleted"`
	// BytesFreed is the number of bytes freed.
	// BytesFreed 是已释放的字节数。
	BytesFreed      int64        `json:"bytes_freed"`
	// Errors is the number of errors encountered.
	// Errors 是遇到的错误数。
	Errors          int          `json:"errors"`
	// DurationMs is the duration in milliseconds.
	// DurationMs 是持续时间（毫秒）。
	DurationMs      int64        `json:"duration_ms"`
	// CleanersRun is the number of cleaners that ran.
	// CleanersRun 是已运行的清理器数量。
	CleanersRun     int          `json:"cleaners_run"`
	// CleanersSkipped is the number of cleaners that were skipped.
	// CleanersSkipped 是被跳过的清理器数量。
	CleanersSkipped int          `json:"cleaners_skipped"`
	// CleanersErrored is the number of cleaners that errored.
	// CleanersErrored 是出错的清理器数量。
	CleanersErrored int          `json:"cleaners_errored"`
	// TS is the timestamp of the event.
	// TS 是事件的时间戳。
	TS              time.Time    `json:"ts"`
}

// MarshalJSON implements custom JSON marshaling for Event.
// MarshalJSON 实现 Event 的自定义 JSON 序列化。
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

// boolPtr returns a pointer to the given bool value.
// boolPtr 返回给定 bool 值的指针。
func boolPtr(v bool) *bool    { return &v }

// intPtr returns a pointer to the given int value.
// intPtr 返回给定 int 值的指针。
func intPtr(v int) *int       { return &v }

// int64Ptr returns a pointer to the given int64 value.
// int64Ptr 返回给定 int64 值的指针。
func int64Ptr(v int64) *int64 { return &v }
