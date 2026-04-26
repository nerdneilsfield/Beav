package json

import (
	"encoding/json"
	"io"
	"sync"

	"github.com/dengqi/beav/internal/cleaner/model"
)

type Renderer struct {
	mu sync.Mutex
	w  io.Writer
}

func New(w io.Writer) *Renderer {
	return &Renderer{w: w}
}

func (r *Renderer) Render(e model.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()

	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	_, _ = r.w.Write(b)
	_, _ = r.w.Write([]byte("\n"))
}

func (r *Renderer) Close() error {
	return nil
}
