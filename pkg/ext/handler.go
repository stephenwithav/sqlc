package ext

import (
	"context"

	"github.com/stephenwithav/sqlc/pkg/plugin"
	"github.com/stephenwithav/template"
)

type Handler interface {
	Generate(context.Context, *plugin.CodeGenRequest, ...template.Option) (*plugin.CodeGenResponse, error)
}

type wrapper struct {
	fn func(context.Context, *plugin.CodeGenRequest, ...template.Option) (*plugin.CodeGenResponse, error)
}

func (w *wrapper) Generate(ctx context.Context, req *plugin.CodeGenRequest, options ...template.Option) (*plugin.CodeGenResponse, error) {
	return w.fn(ctx, req)
}

func HandleFunc(fn func(context.Context, *plugin.CodeGenRequest, ...template.Option) (*plugin.CodeGenResponse, error)) Handler {
	return &wrapper{fn}
}
