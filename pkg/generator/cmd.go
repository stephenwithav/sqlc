package generator

import (
	"context"
	"io"

	"github.com/stephenwithav/sqlc/pkg/plugin"
	"github.com/stephenwithav/template"
)

type Option struct {
	templateOptions  []template.Option
	filesPerTemplate map[string]string
}

// SQLToGo transforms a sqlc.yaml-formatted io.Reader into the appropriate Go
// code.
//
// Returns a map whose keys are the output filenames and whose values are the
// file contents.
func SQLToGo(sql io.Reader, option *Option) (map[string]string, []*plugin.CodeGenRequest, error) {
	return Generate(context.Background(), sql, option)
}
