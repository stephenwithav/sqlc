// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.14.0
// source: query.sql

package querytest

import (
	"context"
)

const joinTextArray = `-- name: JoinTextArray :many
SELECT bar.info
FROM foo
JOIN bar ON foo.bar = bar.id
`

func (q *Queries) JoinTextArray(ctx context.Context) ([][]string, error) {
	rows, err := q.db.Query(ctx, joinTextArray)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items [][]string
	for rows.Next() {
		var info []string
		if err := rows.Scan(&info); err != nil {
			return nil, err
		}
		items = append(items, info)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}
