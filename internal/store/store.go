package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"github.com/Kerberos255/X-port/internal/model"
)

type Store struct{ db *sql.DB }

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000;`); err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }
func (s *Store) Ping() error  { return s.db.Ping() }

func (s *Store) migrate() error {
	if _, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS accounts (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  disabled_reason TEXT NOT NULL DEFAULT '',
  listen TEXT NOT NULL DEFAULT '',
  port INTEGER NOT NULL UNIQUE CHECK(port > 0 AND port <= 65535),
  protocol TEXT NOT NULL,
  settings_json TEXT NOT NULL,
  stream_settings_json TEXT NOT NULL DEFAULT '{}',
  sniffing_json TEXT NOT NULL DEFAULT '{}',
  tag TEXT NOT NULL UNIQUE,
  up_bytes INTEGER NOT NULL DEFAULT 0,
  down_bytes INTEGER NOT NULL DEFAULT 0,
  quota_bytes INTEGER NOT NULL DEFAULT 0,
  all_time_bytes INTEGER NOT NULL DEFAULT 0,
  expiry_time INTEGER NOT NULL DEFAULT 0,
  monthly_reset INTEGER NOT NULL DEFAULT 0,
  last_monthly_reset TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS admins (
  username TEXT PRIMARY KEY,
  password_hash TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);`); err != nil {
		return err
	}
	// Upgrade databases created by early X-port builds in place.
	for _, c := range []struct{ name, ddl string }{
		{"disabled_reason", `disabled_reason TEXT NOT NULL DEFAULT ''`},
		{"monthly_reset", `monthly_reset INTEGER NOT NULL DEFAULT 0`},
		{"last_monthly_reset", `last_monthly_reset TEXT NOT NULL DEFAULT ''`},
	} {
		if err := s.ensureAccountColumn(c.name, c.ddl); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ensureAccountColumn(name, ddl string) error {
	rows, err := s.db.Query(`PRAGMA table_info(accounts)`)
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var cid, notnull, pk int
		var column, typ string
		var dflt any
		if err := rows.Scan(&cid, &column, &typ, &notnull, &dflt, &pk); err != nil {
			rows.Close()
			return err
		}
		if column == name {
			found = true
		}
	}
	err = rows.Close()
	if err != nil {
		return err
	}
	if found {
		return nil
	}
	_, err = s.db.Exec(`ALTER TABLE accounts ADD COLUMN ` + ddl)
	return err
}

func (s *Store) Accounts() ([]model.Account, error) {
	return scanAccounts(s.db.Query(`SELECT id,name,enabled,disabled_reason,listen,port,protocol,settings_json,stream_settings_json,sniffing_json,tag,up_bytes,down_bytes,quota_bytes,all_time_bytes,expiry_time,monthly_reset,last_monthly_reset,created_at,updated_at FROM accounts ORDER BY id ASC`))
}

func scanAccounts(rows *sql.Rows, err error) ([]model.Account, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]model.Account, 0)
	for rows.Next() {
		var a model.Account
		if err := rows.Scan(&a.ID, &a.Name, &a.Enabled, &a.DisabledReason, &a.Listen, &a.Port, &a.Protocol, &a.SettingsJSON, &a.StreamSettingsJSON, &a.SniffingJSON, &a.Tag, &a.UpBytes, &a.DownBytes, &a.QuotaBytes, &a.AllTimeBytes, &a.ExpiryTime, &a.MonthlyReset, &a.LastMonthlyReset, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ReplaceAccounts atomically replaces the account set. Existing positive IDs are
// preserved; ID 0 lets SQLite allocate a new ID.
func (s *Store) ReplaceAccounts(accounts []model.Account) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM accounts`); err != nil {
		return err
	}
	if err := insertAccountsTx(tx, accounts); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) SetAdmin(username, passwordHash string) error {
	if username == "" || passwordHash == "" {
		return errors.New("username and password hash are required")
	}
	now := time.Now().UnixMilli()
	_, err := s.db.Exec(`INSERT INTO admins(username,password_hash,created_at,updated_at) VALUES(?,?,?,?) ON CONFLICT(username) DO UPDATE SET password_hash=excluded.password_hash,updated_at=excluded.updated_at`, username, passwordHash, now, now)
	return err
}

func (s *Store) ReplaceAdmins(admins []model.Admin) error {
	if len(admins) == 0 {
		return errors.New("at least one admin is required")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM admins`); err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	for _, a := range admins {
		if a.Username == "" || a.PasswordHash == "" {
			return errors.New("admin username and password hash are required")
		}
		if _, err := tx.Exec(`INSERT INTO admins(username,password_hash,created_at,updated_at) VALUES(?,?,?,?)`, a.Username, a.PasswordHash, now, now); err != nil {
			return fmt.Errorf("insert admin %q: %w", a.Username, err)
		}
	}
	return tx.Commit()
}

func (s *Store) Admin(username string) (model.Admin, error) {
	var a model.Admin
	err := s.db.QueryRow(`SELECT username,password_hash FROM admins WHERE username=?`, username).Scan(&a.Username, &a.PasswordHash)
	return a, err
}

func (s *Store) SetSetting(key, value string) error {
	if key == "" {
		return errors.New("setting key is required")
	}
	_, err := s.db.Exec(`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

func (s *Store) Setting(key string) (string, bool, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&value)
	if err == nil {
		return value, true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return "", false, err
}
