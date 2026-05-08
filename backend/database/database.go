package database

import (
	"embed"
	"fmt"
	"log"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

func Open(path string) (*sqlx.DB, error) {
	// modernc.org/sqlite uses _pragma=... for pragmas; _fk=true is the
	// mattn/go-sqlite3 syntax and is silently ignored here. Without
	// foreign_keys=on, ON DELETE CASCADE does NOT fire and you get orphan
	// rows in server_cache after deleting a server (visible as
	// "online: 3 / total: 2" on the dashboard).
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sqlx.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite: single writer
	db.SetMaxIdleConns(1)

	if err := migrate(db); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	// One-time cleanup of any orphans left over from before the fk fix.
	if _, err := db.Exec(`DELETE FROM server_cache WHERE server_id NOT IN (SELECT id FROM servers)`); err != nil {
		log.Printf("orphan cleanup: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM jobs WHERE server_id NOT IN (SELECT id FROM servers)`); err != nil {
		log.Printf("orphan cleanup: %v", err)
	}

	return db, nil
}

func migrate(db *sqlx.DB) error {
	files, err := migrations.ReadDir("migrations")
	if err != nil {
		return err
	}
	for _, f := range files {
		sql, err := migrations.ReadFile("migrations/" + f.Name())
		if err != nil {
			return err
		}
		if _, err := db.Exec(string(sql)); err != nil {
			return fmt.Errorf("migration %s: %w", f.Name(), err)
		}
		log.Printf("migration applied: %s", f.Name())
	}
	return nil
}
