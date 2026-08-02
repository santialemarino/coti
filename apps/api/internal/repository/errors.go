package repository

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// uniqueViolation is the SQLSTATE PostgreSQL raises when a unique constraint rejects a row.
const uniqueViolation = "23505"

// isUniqueViolation reports whether err is a uniqueness failure on the named constraint.
// The name matters: a table with several unique constraints would otherwise report the
// wrong conflict to the caller.
func isUniqueViolation(err error, name string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation && pgErr.ConstraintName == name
}
