package golang

// key: templateName; value: output.go
var defaultFilenamePerTemplate = map[string]string{
	"dbFile":        "db.go",
	"modelsFile":    "models.go",
	"interfaceFile": "querier.go",
	"copyfromFile":  "copyfrom.go",
	"batchFile":     "batch.go",
}
