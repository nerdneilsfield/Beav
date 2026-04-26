package ui

import "github.com/dengqi/beav/internal/cleaner/model"

type Renderer interface {
	Render(model.Event)
	Close() error
}
