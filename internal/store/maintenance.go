package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Kerberos255/X-port/internal/model"
)

type Snapshot struct {
	Version   int               `json:"version"`
	CreatedAt int64             `json:"createdAt"`
	Accounts  []model.Account   `json:"accounts"`
	Admins    []model.Admin     `json:"admins"`
	Settings  map[string]string `json:"settings"`
}

func (s *Store) AddTraffic(tag string, up, down int64) error {
	if tag == "" || (up <= 0 && down <= 0) {
		return nil
	}
	if up < 0 || down < 0 {
		return errors.New("traffic delta cannot be negative")
	}
	_, err := s.db.Exec(`UPDATE accounts SET up_bytes=up_bytes+?,down_bytes=down_bytes+?,all_time_bytes=all_time_bytes+?,updated_at=? WHERE tag=?`, up, down, up+down, time.Now().UnixMilli(), tag)
	return err
}

func (s *Store) ResetTraffic(id int64) error {
	res, err := s.db.Exec(`UPDATE accounts SET up_bytes=0,down_bytes=0,updated_at=? WHERE id=?`, time.Now().UnixMilli(), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("account not found")
	}
	return nil
}

func (s *Store) Admins() ([]model.Admin, error) {
	rows, err := s.db.Query(`SELECT username,password_hash FROM admins ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Admin
	for rows.Next() {
		var a model.Admin
		if err := rows.Scan(&a.Username, &a.PasswordHash); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) Settings() (map[string]string, error) {
	rows, err := s.db.Query(`SELECT key,value FROM settings ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

func (s *Store) Snapshot() (Snapshot, error) {
	accounts, err := s.Accounts()
	if err != nil {
		return Snapshot{}, err
	}
	admins, err := s.Admins()
	if err != nil {
		return Snapshot{}, err
	}
	settings, err := s.Settings()
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{Version: 1, CreatedAt: time.Now().UnixMilli(), Accounts: accounts, Admins: admins, Settings: settings}, nil
}

func (s *Store) ReplaceSnapshot(snapshot Snapshot) error {
	if len(snapshot.Admins) == 0 {
		return errors.New("snapshot has no administrator")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM accounts`); err != nil {
		return err
	}
	if err := insertAccountsTx(tx, snapshot.Accounts); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM admins`); err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	for _, a := range snapshot.Admins {
		if a.Username == "" || a.PasswordHash == "" {
			return errors.New("snapshot contains invalid administrator")
		}
		if _, err := tx.Exec(`INSERT INTO admins(username,password_hash,created_at,updated_at) VALUES(?,?,?,?)`, a.Username, a.PasswordHash, now, now); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`DELETE FROM settings`); err != nil {
		return err
	}
	for k, v := range snapshot.Settings {
		if k == "" {
			continue
		}
		if _, err := tx.Exec(`INSERT INTO settings(key,value) VALUES(?,?)`, k, v); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func insertAccountsTx(tx *sql.Tx, accounts []model.Account) error {
	now := time.Now().UnixMilli()
	for _, a := range accounts {
		if a.Port < 1 || a.Port > 65535 {
			return fmt.Errorf("invalid port %d", a.Port)
		}
		if a.CreatedAt == 0 {
			a.CreatedAt = now
		}
		if a.UpdatedAt == 0 {
			a.UpdatedAt = now
		}
		var q string
		var args []any
		if a.ID > 0 {
			q = `INSERT INTO accounts(id,name,enabled,disabled_reason,listen,port,protocol,settings_json,stream_settings_json,sniffing_json,tag,up_bytes,down_bytes,quota_bytes,all_time_bytes,expiry_time,monthly_reset,last_monthly_reset,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
			args = []any{a.ID, a.Name, a.Enabled, a.DisabledReason, a.Listen, a.Port, a.Protocol, a.SettingsJSON, a.StreamSettingsJSON, a.SniffingJSON, a.Tag, a.UpBytes, a.DownBytes, a.QuotaBytes, a.AllTimeBytes, a.ExpiryTime, a.MonthlyReset, a.LastMonthlyReset, a.CreatedAt, a.UpdatedAt}
		} else {
			q = `INSERT INTO accounts(name,enabled,disabled_reason,listen,port,protocol,settings_json,stream_settings_json,sniffing_json,tag,up_bytes,down_bytes,quota_bytes,all_time_bytes,expiry_time,monthly_reset,last_monthly_reset,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
			args = []any{a.Name, a.Enabled, a.DisabledReason, a.Listen, a.Port, a.Protocol, a.SettingsJSON, a.StreamSettingsJSON, a.SniffingJSON, a.Tag, a.UpBytes, a.DownBytes, a.QuotaBytes, a.AllTimeBytes, a.ExpiryTime, a.MonthlyReset, a.LastMonthlyReset, a.CreatedAt, a.UpdatedAt}
		}
		if _, err := tx.Exec(q, args...); err != nil {
			return fmt.Errorf("insert account %q: %w", a.Name, err)
		}
	}
	return nil
}

func (s *Store) ApplySettings(values map[string]string, oldUsername, newUsername, passwordHash string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for k, v := range values {
		if k == "" {
			continue
		}
		if _, err := tx.Exec(`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, k, v); err != nil {
			return err
		}
	}
	if oldUsername != "" {
		if newUsername == "" || passwordHash == "" {
			return errors.New("admin username and password hash are required")
		}
		var count int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM admins WHERE username=?`, oldUsername).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			return errors.New("admin not found")
		}
		if oldUsername != newUsername {
			if err := tx.QueryRow(`SELECT COUNT(*) FROM admins WHERE username=?`, newUsername).Scan(&count); err != nil {
				return err
			}
			if count != 0 {
				return errors.New("admin username already exists")
			}
			if _, err := tx.Exec(`DELETE FROM admins WHERE username=?`, oldUsername); err != nil {
				return err
			}
			if _, err := tx.Exec(`INSERT INTO admins(username,password_hash,created_at,updated_at) VALUES(?,?,?,?)`, newUsername, passwordHash, time.Now().UnixMilli(), time.Now().UnixMilli()); err != nil {
				return err
			}
		} else if _, err := tx.Exec(`UPDATE admins SET password_hash=?,updated_at=? WHERE username=?`, passwordHash, time.Now().UnixMilli(), oldUsername); err != nil {
			return err
		}
	}
	return tx.Commit()
}
