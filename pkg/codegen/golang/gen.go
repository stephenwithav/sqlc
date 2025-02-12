package golang

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/format"
	"path/filepath"
	"strings"

	"github.com/stephenwithav/sqlc/pkg/codegen/sdk"
	"github.com/stephenwithav/sqlc/pkg/metadata"
	"github.com/stephenwithav/sqlc/pkg/plugin"
	"github.com/stephenwithav/template"
)

type tmplCtx struct {
	Q           string
	Package     string
	SQLDriver   SQLDriver
	Enums       []Enum
	Structs     []Struct
	GoQueries   []Query
	SqlcVersion string

	// TODO: Race conditions
	SourceName string

	EmitJSONTags              bool
	EmitDBTags                bool
	EmitPreparedQueries       bool
	EmitInterface             bool
	EmitEmptySlices           bool
	EmitMethodsWithDBArgument bool
	EmitEnumValidMethod       bool
	EmitAllEnumValues         bool
	UsesCopyFrom              bool
	UsesBatch                 bool
}

func (t *tmplCtx) OutputQuery(sourceName string) bool {
	return t.SourceName == sourceName
}

func Generate(ctx context.Context, req *plugin.CodeGenRequest, options []template.Option, templateFiles map[string]string) (*plugin.CodeGenResponse, error) {
	enums := buildEnums(req)
	structs := buildStructs(req)
	queries, err := buildQueries(req, structs)
	if err != nil {
		return nil, err
	}
	if len(templateFiles) == 0 {
		templateFiles = make(map[string]string, len(defaultFilenamePerTemplate))
		for key, val := range defaultFilenamePerTemplate {
			templateFiles[key] = val
		}
	}
	return generate(req, enums, structs, queries, options, templateFiles)
}

func generate(req *plugin.CodeGenRequest, enums []Enum, structs []Struct, queries []Query, options []template.Option, templateFiles map[string]string) (*plugin.CodeGenResponse, error) {
	i := &importer{
		Settings: req.Settings,
		Queries:  queries,
		Enums:    enums,
		Structs:  structs,
	}

	// Every template can use these funcs.
	// Ensure they're loaded first.
	options = append([]template.Option{template.Funcs(template.FuncMap{
		"lowerTitle": sdk.LowerTitle,
		"comment":    sdk.DoubleSlashComment,
		"escape":     sdk.EscapeBacktick,
		"imports":    i.Imports,
		"hasPrefix":  strings.HasPrefix,
	})}, options...)

	// Default behavior if no options are passed in.
	if len(options) == 1 {
		options = append(options, template.ParseFS(
			templates,
			"templates/*.tmpl",
			"templates/*/*.tmpl",
		))
	}

	tmpl := template.Must(template.New("table", options...))

	golang := req.Settings.Go
	tctx := tmplCtx{
		EmitInterface:             golang.EmitInterface,
		EmitJSONTags:              golang.EmitJsonTags,
		EmitDBTags:                golang.EmitDbTags,
		EmitPreparedQueries:       golang.EmitPreparedQueries,
		EmitEmptySlices:           golang.EmitEmptySlices,
		EmitMethodsWithDBArgument: golang.EmitMethodsWithDbArgument,
		EmitEnumValidMethod:       golang.EmitEnumValidMethod,
		EmitAllEnumValues:         golang.EmitAllEnumValues,
		UsesCopyFrom:              usesCopyFrom(queries),
		UsesBatch:                 usesBatch(queries),
		SQLDriver:                 parseDriver(golang.SqlPackage),
		Q:                         "`",
		Package:                   filepath.Base(golang.Package),
		GoQueries:                 queries,
		Enums:                     enums,
		Structs:                   structs,
		SqlcVersion:               req.SqlcVersion,
	}

	if tctx.UsesCopyFrom && !tctx.SQLDriver.IsPGX() {
		return nil, errors.New(":copyfrom is only supported by pgx")
	}

	if tctx.UsesBatch && !tctx.SQLDriver.IsPGX() {
		return nil, errors.New(":batch* commands are only supported by pgx")
	}

	output := map[string]string{}

	execute := func(name, templateName string) error {
		var b bytes.Buffer
		w := bufio.NewWriter(&b)
		tctx.SourceName = name
		err := tmpl.ExecuteTemplate(w, templateName, &tctx)
		w.Flush()
		if err != nil {
			return err
		}
		code, err := format.Source(b.Bytes())
		if err != nil {
			fmt.Println(b.String())
			return fmt.Errorf("source error: %w", err)
		}

		if templateName == "queryFile" && golang.OutputFilesSuffix != "" {
			name += golang.OutputFilesSuffix
		}

		if !strings.HasSuffix(name, ".go") {
			name += ".go"
		}
		output[name] = string(code)
		return nil
	}

	if querierFileName, ok := templateFiles["interfaceFile"]; ok && tctx.EmitInterface {
		if err := execute(querierFileName, "interfaceFile"); err != nil {
			return nil, err
		}
		delete(templateFiles, "interfaceFile")
	}
	if copyfromFileName, ok := templateFiles["copyfromFile"]; ok && tctx.UsesCopyFrom {
		if err := execute(copyfromFileName, "copyfromFile"); err != nil {
			return nil, err
		}
		delete(templateFiles, "copyfromFile")
	}
	if batchFileName, ok := templateFiles["batchFile"]; ok && tctx.UsesBatch {
		if err := execute(batchFileName, "batchFile"); err != nil {
			return nil, err
		}
		delete(templateFiles, "batchFile")
	}

	if golang.OutputDbFileName != "" {
		templateFiles["dbFile"] = golang.OutputDbFileName
	}
	if golang.OutputModelsFileName != "" {
		templateFiles["modelsFile"] = golang.OutputModelsFileName
	}

	for templateName, outputFile := range templateFiles {
		if err := execute(outputFile, templateName); err != nil {
			return nil, err
		}
	}

	files := map[string]struct{}{}
	for _, gq := range queries {
		files[gq.SourceName] = struct{}{}
	}

	for source := range files {
		if err := execute(source, "queryFile"); err != nil {
			return nil, err
		}
	}
	resp := plugin.CodeGenResponse{}

	for filename, code := range output {
		resp.Files = append(resp.Files, &plugin.File{
			Name:     filename,
			Contents: []byte(code),
		})
	}

	return &resp, nil
}

func usesCopyFrom(queries []Query) bool {
	for _, q := range queries {
		if q.Cmd == metadata.CmdCopyFrom {
			return true
		}
	}
	return false
}

func usesBatch(queries []Query) bool {
	for _, q := range queries {
		for _, cmd := range []string{metadata.CmdBatchExec, metadata.CmdBatchMany, metadata.CmdBatchOne} {
			if q.Cmd == cmd {
				return true
			}
		}
	}
	return false
}
