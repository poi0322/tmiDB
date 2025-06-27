package database

import "database/sql"

// DBTX is a common interface for *sql.DB and *sql.Tx.
// This allows for functions to be used both within and outside of transactions.
type DBTX interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	QueryRow(query string, args ...interface{}) *sql.Row
	Query(query string, args ...interface{}) (*sql.Rows, error)
}
