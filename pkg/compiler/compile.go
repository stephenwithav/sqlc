package compiler

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/stephenwithav/sqlc/pkg/metadata"
	"github.com/stephenwithav/sqlc/pkg/migrations"
	"github.com/stephenwithav/sqlc/pkg/multierr"
	"github.com/stephenwithav/sqlc/pkg/opts"
	"github.com/stephenwithav/sqlc/pkg/sql/ast"
	"github.com/stephenwithav/sqlc/pkg/sql/sqlerr"
)

// TODO: Rename this interface Engine
type Parser interface {
	Parse(io.Reader) ([]ast.Statement, error)
	CommentSyntax() metadata.CommentSyntax
	IsReservedKeyword(string) bool
}

// copied over from gen.go
func structName(name string) string {
	out := ""
	for _, p := range strings.Split(name, "_") {
		if p == "id" {
			out += "ID"
		} else {
			out += strings.Title(p)
		}
	}
	return out
}

var identPattern = regexp.MustCompile("[^a-zA-Z0-9_]+")

func enumValueName(value string) string {
	name := ""
	id := strings.Replace(value, "-", "_", -1)
	id = strings.Replace(id, ":", "_", -1)
	id = strings.Replace(id, "/", "_", -1)
	id = identPattern.ReplaceAllString(id, "")
	for _, part := range strings.Split(id, "_") {
		name += strings.Title(part)
	}
	return name
}

// end copypasta
func (c *Compiler) parseCatalog(schemas []string) error {
	// schemas[0] contains the schemas
	merr := multierr.New()
	for _, schema := range schemas {
		contents := migrations.RemoveRollbackStatements(schema)
		stmts, err := c.parser.Parse(strings.NewReader(contents))
		if err != nil {
			merr.Add(schema, contents, 0, err)
			continue
		}
		for i := range stmts {
			if err := c.catalog.Update(stmts[i], c); err != nil {
				merr.Add(schema, contents, stmts[i].Pos(), err)
				continue
			}
		}
	}
	if len(merr.Errs()) > 0 {
		return merr
	}
	return nil
}

func (c *Compiler) parseQueries(o opts.Parser) (*Result, error) {
	var q []*Query
	merr := multierr.New()
	set := map[string]struct{}{}
	for _, queryFromYaml := range c.conf.Queries {
		src := string(queryFromYaml)
		stmts, err := c.parser.Parse(strings.NewReader(src))
		if err != nil {
			merr.Add(queryFromYaml, src, 0, err)
			continue
		}
		for _, stmt := range stmts {
			query, err := c.parseQuery(stmt.Raw, src, o)
			if err == ErrUnsupportedStatementType {
				continue
			}
			if err != nil {
				var e *sqlerr.Error
				loc := stmt.Raw.Pos()
				if errors.As(err, &e) && e.Location != 0 {
					loc = e.Location
				}
				merr.Add(queryFromYaml, src, loc, err)
				continue
			}
			if query.Name != "" {
				if _, exists := set[query.Name]; exists {
					merr.Add(queryFromYaml, src, stmt.Raw.Pos(), fmt.Errorf("duplicate query name: %s", query.Name))
					continue
				}
				set[query.Name] = struct{}{}
			}
			query.Filename = "queries"
			if query != nil {
				q = append(q, query)
			}
		}
	}
	if len(merr.Errs()) > 0 {
		return nil, merr
	}
	if len(q) == 0 {
		return nil, fmt.Errorf("no queries contained in paths %s", strings.Join(c.conf.Queries, ","))
	}
	return &Result{
		Catalog: c.catalog,
		Queries: q,
	}, nil
}
