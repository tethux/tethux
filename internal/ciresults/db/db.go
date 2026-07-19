package db

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	migratesqlite "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "modernc.org/sqlite"

	"github.com/0xveya/tethux/internal/ciresults/db/sqlc"
	"github.com/0xveya/tethux/tools/ci-results/viewer/handlers/types"
)

//go:embed migrations/*.sql
var migrations embed.FS

type Store struct {
	*sqlc.Queries
	DB *sql.DB
}

func NewStore(dbPath string) (_ *Store, returnErr error) {
	if dbPath == "" {
		dbPath = filepath.Join("data", "ci", "ci-res.db")
	}

	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0o750); err != nil {
		return nil, fmt.Errorf("create database directory %q: %w", dbDir, err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open SQLite database %q: %w", dbPath, err)
	}

	defer func() {
		if returnErr != nil {
			_ = db.Close()
		}
	}()

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	ctx := context.Background()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("connect to SQLite database %q: %w", dbPath, err)
	}

	const setupSQL = `
PRAGMA foreign_keys = ON;
PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;
PRAGMA busy_timeout = 5000;
`

	if _, err := db.ExecContext(ctx, setupSQL); err != nil {
		return nil, fmt.Errorf("configure SQLite database: %w", err)
	}

	if err := migrateUp(db); err != nil {
		return nil, fmt.Errorf("migrate database: %w", err)
	}

	return &Store{
		Queries: sqlc.New(db),
		DB:      db,
	}, nil
}

func migrateUp(db *sql.DB) error {
	legacy, err := prepareLegacyMigrationState(context.Background(), db)
	if err != nil {
		return err
	}
	source, err := iofs.New(migrations, "migrations")
	if err != nil {
		return fmt.Errorf("open embedded migrations: %w", err)
	}
	driver, err := migratesqlite.WithInstance(db, &migratesqlite.Config{})
	if err != nil {
		return fmt.Errorf("create SQLite migration driver: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", source, "sqlite", driver)
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}
	if legacy {
		if err := m.Force(1); err != nil {
			return fmt.Errorf("baseline legacy schema: %w", err)
		}
		return nil
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
}

func prepareLegacyMigrationState(ctx context.Context, db *sql.DB) (bool, error) {
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(schema_migrations)`)
	if err != nil {
		return false, fmt.Errorf("inspect migration table: %w", err)
	}
	defer rows.Close()

	found, hasDirty := false, false
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, err
		}
		found = true
		if name == "dirty" {
			hasDirty = true
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	if !found || hasDirty {
		return false, nil
	}
	if _, err := db.ExecContext(ctx, `DROP TABLE schema_migrations`); err != nil {
		return false, fmt.Errorf("remove legacy migration table: %w", err)
	}
	return true, nil
}

func (s *Store) GetSchema(ctx context.Context) (string, error) {
	query := `
		SELECT sql
		FROM sqlite_master
		WHERE type IN ('table', 'view', 'trigger')
		  AND name NOT LIKE 'sqlite_%'
		  AND name NOT IN (
			SELECT name FROM pragma_table_list WHERE type = 'shadow'
		  )
		  AND sql IS NOT NULL
		ORDER BY
			CASE type
				WHEN 'table' THEN 1
				WHEN 'view' THEN 2
				WHEN 'trigger' THEN 3
			END,
			name;
	`

	rows, err := s.DB.QueryContext(ctx, query)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var schema strings.Builder
	schema.WriteString("PRAGMA foreign_keys = ON;\n\n")

	for rows.Next() {
		var sqlStmt sql.NullString
		if err := rows.Scan(&sqlStmt); err != nil {
			return "", err
		}
		if sqlStmt.Valid && sqlStmt.String != "" {
			schema.WriteString(sqlStmt.String)
			schema.WriteString(";\n\n")
		}
	}

	if err := rows.Err(); err != nil {
		return "", err
	}

	return strings.TrimSpace(schema.String()), nil
}

func (s *Store) Close() error {
	if s == nil || s.DB == nil {
		return nil
	}

	return s.DB.Close()
}

func (s *Store) GetSchemaInfo(ctx context.Context) (types.DBSchemaInfo, error) {
	query := `select s.type, s.name, c.name as column_name, c.type as column_type from sqlite_master as s join pragma_table_info(s.name) as c where s.type in ('table', 'view') and s.name not like 'sqlite_%' order by s.type, s.name, c.cid;`
	rows, err := s.DB.QueryContext(ctx, query)
	if err != nil {
		return types.DBSchemaInfo{}, err
	}
	defer rows.Close()

	var objects []types.SchemaObject
	for rows.Next() {
		var kind types.SchemaObjectKind
		var name, columnName, columnType string
		if err := rows.Scan(&kind, &name, &columnName, &columnType); err != nil {
			return types.DBSchemaInfo{}, err
		}
		objects = append(objects, types.SchemaObject{
			Name:    name,
			Kind:    kind,
			Columns: []types.SchemaColumn{{Name: columnName, Type: columnType}},
		})
	}

	if err := rows.Err(); err != nil {
		return types.DBSchemaInfo{}, err
	}

	return types.DBSchemaInfo{Objects: objects}, nil
}
