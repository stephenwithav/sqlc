package generator

import (
	"context"
	"io"
)

// SQLToGo transforms a sqlc.yaml-formatted io.Reader into the appropriate Go
// code.
//
// Returns a map whose keys are the output filenames and whose values are the
// file contents.
func SQLToGo(sql io.Reader) (map[string]string, error) {
	return Generate(context.Background(), sql)
}
