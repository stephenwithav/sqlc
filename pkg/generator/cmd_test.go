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
    queries: |
      -- name: GetAuthor :one
         SELECT * FROM authors
         WHERE id = $1 LIMIT 1;

      -- name: ListAuthors :many
         SELECT * FROM authors
         ORDER BY name;

      -- name: CreateAuthor :one
         INSERT INTO authors (
           name, bio
         ) VALUES (
           $1, $2
         )
         RETURNING *;

      -- name: DeleteAuthor :exec
         DELETE FROM authors
         WHERE id = $1;
    schema: |
      CREATE TABLE authors (
        id   BIGSERIAL PRIMARY KEY,
        name text      NOT NULL,
        bio  text
      )
    gen:
      go:
        package: "generator"
        out: "generator"
  - engine: "mysql"
    queries: |
      -- name: GetAuthor :one
         SELECT * FROM authors
         WHERE id = $1 LIMIT 1;

      -- name: ListAuthors :many
         SELECT * FROM authors
         ORDER BY name;

      -- name: CreateAuthor :execresult
         INSERT INTO authors (
           name, bio
         ) VALUES (
           $1, $2
         );

      -- name: DeleteAuthor :exec
         DELETE FROM authors
         WHERE id = $1;
    schema: |
      CREATE TABLE authors (
        id   BIGINT AUTO_INCREMENT PRIMARY KEY,
        name varchar(255)      NOT NULL,
        bio  varchar(255)
      )
    gen:
      go:
        package: "mygenerator"
        out: "mygenerator"
`

	r := strings.NewReader(given)
	sqlmap, err := SQLToGo(r)
	if err != nil {
		log.Println(err)
	}

	for file, _ := range sqlmap {
		log.Println(file)
	}
}
