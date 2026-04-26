// Package ui defines the renderer interface for displaying cleaner events.
// Package ui 定义用于显示清理器事件的渲染器接口。
package ui

import "github.com/dengqi/beav/internal/cleaner/model"

// Renderer is the interface for displaying cleaner events to the user.
// Renderer 是向用户显示清理器事件的接口。
type Renderer interface {
	Render(model.Event)
	Close() error
}
