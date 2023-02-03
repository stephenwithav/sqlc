package generator

import (
	"log"
	"strings"
	"testing"
)

func TestYaml(t *testing.T) {
	given := `
version: "2"
sql:
  - engine: "postgresql"
    queries: "query.sql"
    schema: "schema.sql"
    gen:
      go:
        package: "generator"
        out: "generator"`

	r := strings.NewReader(given)
	_, err := SQLToGo(r)
	if err != nil {
		log.Println(err)
	}
}
