package repository

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// uniqueViolation is the SQLSTATE PostgreSQL raises when a unique constraint or unique
// index rejects a row.
const uniqueViolation = "23505"

// isUniqueViolation reports whether err is a uniqueness failure on the named constraint
// or index.
//
// The name is checked rather than the code alone: a table can carry several unique
// constraints, and a repository that maps any of them to the same domain error would
// report the wrong conflict to the caller.
func isUniqueViolation(err error, name string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation && pgErr.ConstraintName == name
}
