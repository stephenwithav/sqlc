package generator

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/trace"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/stephenwithav/sqlc/pkg/codegen/golang"
	"github.com/stephenwithav/sqlc/pkg/codegen/json"
	"github.com/stephenwithav/sqlc/pkg/compiler"
	"github.com/stephenwithav/sqlc/pkg/config"
	"github.com/stephenwithav/sqlc/pkg/config/convert"
	"github.com/stephenwithav/sqlc/pkg/debug"
	"github.com/stephenwithav/sqlc/pkg/ext"
	"github.com/stephenwithav/sqlc/pkg/ext/process"
	"github.com/stephenwithav/sqlc/pkg/ext/wasm"
	"github.com/stephenwithav/sqlc/pkg/multierr"
	"github.com/stephenwithav/sqlc/pkg/opts"
	"github.com/stephenwithav/sqlc/pkg/plugin"
)

const errMessageNoVersion = `The configuration file must have a version number.
Set the version to 1 or 2 at the top of sqlc.json:

{
  "version": "1"
  ...
}
`

const errMessageUnknownVersion = `The configuration file has an invalid version number.
The supported version can only be "1" or "2".
`

const errMessageNoPackages = `No packages are configured`

func printFileErr(stderr io.Writer, dir string, fileErr *multierr.FileError) {
	filename := strings.TrimPrefix(fileErr.Filename, dir+"/")
	fmt.Fprintf(stderr, "%s:%d:%d: %s\n", filename, fileErr.Line, fileErr.Column, fileErr.Err)
}

type outPair struct {
	Gen    config.SQLGen
	Plugin *config.Codegen

	config.SQL
}

func findPlugin(conf config.Config, name string) (*config.Plugin, error) {
	for _, plug := range conf.Plugins {
		if plug.Name == name {
			return &plug, nil
		}
	}
	return nil, fmt.Errorf("plugin not found")
}

func readConfig(stderr io.Writer, configSource io.Reader) (*config.Config, error) {
	conf, err := config.ParseConfig(configSource)
	if err != nil {
		switch err {
		case config.ErrMissingVersion:
			fmt.Fprintf(stderr, errMessageNoVersion)
		case config.ErrUnknownVersion:
			fmt.Fprintf(stderr, errMessageUnknownVersion)
		case config.ErrNoPackages:
			fmt.Fprintf(stderr, errMessageNoPackages)
		}
		return nil, err
	}

	return &conf, nil
}

func Generate(ctx context.Context, configSource io.Reader) (map[string]string, []*plugin.CodeGenRequest, error) {
	// config.ParseConfig is the magic here. It accepts an io.Reader, which
	// could be a bytes.Reader or strings.NewReader. configPath is really
	// unnecessary.
	conf, err := readConfig(os.Stderr, configSource)
	if err != nil {
		return nil, nil, err
	}

	output := map[string]string{}
	errored := false

	var pairs []outPair
	for _, sql := range conf.SQL {
		if sql.Gen.Go != nil {
			pairs = append(pairs, outPair{
				SQL: sql,
				Gen: config.SQLGen{Go: sql.Gen.Go},
			})
		}
		if sql.Gen.JSON != nil {
			pairs = append(pairs, outPair{
				SQL: sql,
				Gen: config.SQLGen{JSON: sql.Gen.JSON},
			})
		}
		for i, _ := range sql.Codegen {
			pairs = append(pairs, outPair{
				SQL:    sql,
				Plugin: &sql.Codegen[i],
			})
		}
	}

	var m sync.Mutex
	grp, gctx := errgroup.WithContext(ctx)
	grp.SetLimit(runtime.GOMAXPROCS(0))

	stderrs := make([]bytes.Buffer, len(pairs))
	codeGenReqs := make([]*plugin.CodeGenRequest, len(pairs))

	for i, pair := range pairs {
		sql := pair
		errout := &stderrs[i]

		grp.Go(func() error {
			combo := config.Combine(*conf, sql.SQL)
			if sql.Plugin != nil {
				combo.Codegen = *sql.Plugin
			}

			// TODO: This feels like a hack that will bite us later
			joined := make([]string, 0, len(sql.Schema))
			for _, s := range sql.Schema {
				joined = append(joined, s)
			}
			sql.Schema = joined

			joined = make([]string, 0, len(sql.Queries))
			for _, q := range sql.Queries {
				joined = append(joined, q)
			}
			sql.Queries = joined
			// fmt.Printf("Queries: %+v\n", sql.Queries)

			var name, lang string
			parseOpts := opts.Parser{
				Debug: debug.Debug,
			}

			switch {
			case sql.Gen.Go != nil:
				name = combo.Go.Package
				lang = "golang"

			case sql.Plugin != nil:
				lang = fmt.Sprintf("process:%s", sql.Plugin.Plugin)
				name = sql.Plugin.Plugin
			}

			packageRegion := trace.StartRegion(gctx, "package")
			trace.Logf(gctx, "", "name=%s plugin=%s", name, lang)

			result, failed := parse(gctx, sql.SQL, combo, parseOpts, errout)
			if failed {
				packageRegion.End()
				errored = true
				return nil
			}

			// fmt.Printf("result: %+v\n", result.Catalog.Schemas[0].Tables[0].Columns[0].Type.Name)
			out, resp, codeGenReq, err := codegen(gctx, combo, sql, result)
			if err != nil {
				fmt.Fprintf(errout, "# package %s\n", name)
				fmt.Fprintf(errout, "error generating code: %s\n", err)
				errored = true
				packageRegion.End()
				return nil
			}
			codeGenReqs = append(codeGenReqs, codeGenReq)

			// fmt.Printf("resp: %+v\n", resp.GetFiles())
			files := map[string]string{}
			for _, file := range resp.Files {
				files[file.Name] = string(file.Contents)
			}

			m.Lock()
			for n, source := range files {
				filename := filepath.Join(out, n)
				output[filename] = source
			}
			m.Unlock()

			packageRegion.End()
			return nil
		})
	}
	if err := grp.Wait(); err != nil {
		return nil, nil, err
	}
	if errored {
		for i, _ := range stderrs {
			if _, err := io.Copy(os.Stderr, &stderrs[i]); err != nil {
				return nil, nil, err
			}
		}
		return nil, nil, fmt.Errorf("errored")
	}
	return output, codeGenReqs, nil
}

func parse(ctx context.Context, sql config.SQL, combo config.CombinedSettings, parserOpts opts.Parser, stderr io.Writer) (*compiler.Result, bool) {
	defer trace.StartRegion(ctx, "parse").End()
	c := compiler.NewCompiler(sql, combo)
	if err := c.ParseCatalog(sql.Schema); err != nil {
		fmt.Fprintf(stderr, "error parsing schema: %s\n", err)
		return nil, true
	}
	if parserOpts.Debug.DumpCatalog {
		debug.Dump(c.Catalog())
	}
	if err := c.ParseQueries(sql.Queries, parserOpts); err != nil {
		fmt.Fprintf(stderr, "error parsing queries: %s\n", err)
		return nil, true
	}
	return c.Result(), false
}

func codegen(ctx context.Context, combo config.CombinedSettings, sql outPair, result *compiler.Result) (string, *plugin.CodeGenResponse, *plugin.CodeGenRequest, error) {
	defer trace.StartRegion(ctx, "codegen").End()
	req := codeGenRequest(result, combo)
	fmt.Printf("Queriez: %+v\n", req.GetQueries()[0].GetColumns())
	var handler ext.Handler
	var out string
	switch {
	case sql.Gen.Go != nil:
		out = combo.Go.Out
		handler = ext.HandleFunc(golang.Generate)

	case sql.Gen.JSON != nil:
		out = combo.JSON.Out
		handler = ext.HandleFunc(json.Generate)

	case sql.Plugin != nil:
		out = sql.Plugin.Out
		plug, err := findPlugin(combo.Global, sql.Plugin.Plugin)
		if err != nil {
			return "", nil, req, fmt.Errorf("plugin not found: %s", err)
		}

		switch {
		case plug.Process != nil:
			handler = &process.Runner{
				Cmd: plug.Process.Cmd,
			}
		case plug.WASM != nil:
			handler = &wasm.Runner{
				URL:    plug.WASM.URL,
				SHA256: plug.WASM.SHA256,
			}
		default:
			return "", nil, req, fmt.Errorf("unsupported plugin type")
		}

		opts, err := convert.YAMLtoJSON(sql.Plugin.Options)
		if err != nil {
			return "", nil, req, fmt.Errorf("invalid plugin options")
		}
		req.PluginOptions = opts

	default:
		return "", nil, req, fmt.Errorf("missing language backend")
	}
	resp, err := handler.Generate(ctx, req)
	return out, resp, req, err
}
