package compiler

import (
	"github.com/stephenwithav/sqlc/pkg/sql/catalog"
)

type Result struct {
	Catalog *catalog.Catalog
	Queries []*Query
}
