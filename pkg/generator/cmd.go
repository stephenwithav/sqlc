package generator

import (
	"context"
	"io"

	"github.com/stephenwithav/sqlc/pkg/plugin"
)

// TODO: Change SQLToGo's input to SQLCParams, where each file field corresponds
// to an io.Reader.
//
// TODO: Change Generate to accept SQLCParams, read file contents from there.
type SQLCParams struct {
}

// SQLToGo transforms a sqlc.yaml-formatted io.Reader into the appropriate Go
// code.
//
// Returns a map whose keys are the output filenames and whose values are the
// file contents.
func SQLToGo(sql io.Reader) (map[string]string, []*plugin.CodeGenRequest, error) {
	return Generate(context.Background(), sql)
}
