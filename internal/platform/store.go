package platform

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

type Store struct {
	db   *sql.DB
	Path string
}
type SavedConnection struct {
	Connection Connection
	SecretRef  string
}
type HistoryFilter struct {
	ConnectionID string `json:"connectionId"`
	Job          string `json:"job"`
	Status       string `json:"status"`
	Since        int64  `json:"since"`
	Until        int64  `json:"until"`
	Offset       int    `json:"offset"`
}
type HistoryPage struct {
	Builds []Build `json:"builds"`
	Total  int     `json:"total"`
	Size   int64   `json:"size"`
}

func OpenStore(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	var version int
	if err = db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		db.Close()
		return nil, err
	}
	if version > 1 {
		db.Close()
		return nil, errors.New("Database was created by a newer version of the app")
	}
	s := &Store{db: db, Path: path}
	_, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;
CREATE TABLE IF NOT EXISTS connections(id TEXT PRIMARY KEY, config TEXT NOT NULL, secret_ref TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS builds(connection_id TEXT NOT NULL,job TEXT NOT NULL,number INTEGER NOT NULL,started INTEGER NOT NULL,duration INTEGER NOT NULL,status TEXT NOT NULL,raw_status TEXT NOT NULL,commit_sha TEXT NOT NULL,url TEXT NOT NULL,observed INTEGER NOT NULL,PRIMARY KEY(connection_id,job,number,started));
CREATE INDEX IF NOT EXISTS builds_started ON builds(started DESC);
CREATE INDEX IF NOT EXISTS builds_running ON builds(connection_id,status);
CREATE TABLE IF NOT EXISTS preferences(key TEXT PRIMARY KEY,value TEXT NOT NULL);
PRAGMA user_version=1;`)
	if err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}
func (s *Store) Close() error { return s.db.Close() }
func (s *Store) Connections() ([]SavedConnection, error) {
	rows, err := s.db.Query("SELECT config,secret_ref FROM connections ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SavedConnection{}
	for rows.Next() {
		var raw, ref string
		if err = rows.Scan(&raw, &ref); err != nil {
			return nil, err
		}
		var c Connection
		if err = json.Unmarshal([]byte(raw), &c); err != nil {
			return nil, err
		}
		out = append(out, SavedConnection{c, ref})
	}
	return out, rows.Err()
}
func (s *Store) SaveConnection(c Connection, ref string) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("INSERT INTO connections VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET config=excluded.config,secret_ref=excluded.secret_ref", c.ID, string(b), ref)
	return err
}
func (s *Store) DeleteConnection(id string) error {
	_, err := s.db.Exec("DELETE FROM connections WHERE id=?", id)
	return err
}

func (s *Store) Known(b Build) (bool, error) {
	var count int
	err := s.db.QueryRow("SELECT count(*) FROM builds WHERE connection_id=? AND job=? AND number=? AND started=?", b.ConnectionID, b.Job, b.Number, b.Started).Scan(&count)
	return count > 0, err
}

func (s *Store) SaveBuilds(builds []Build) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO builds VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(connection_id,job,number,started) DO UPDATE SET duration=excluded.duration,status=excluded.status,raw_status=excluded.raw_status,commit_sha=excluded.commit_sha,url=excluded.url,observed=excluded.observed WHERE excluded.observed >= builds.observed AND NOT (builds.status NOT IN ('RUNNING','UNKNOWN','UNCONFIRMED') AND excluded.status IN ('RUNNING','UNKNOWN','UNCONFIRMED'))`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, b := range builds {
		if b.Started <= 0 || b.Number < 1 {
			return errors.New("Build has no stable timestamp or number; not archived")
		}
		if _, err = stmt.Exec(b.ConnectionID, b.Job, b.Number, b.Started, b.Duration, b.Status, b.RawStatus, b.Commit, b.URL, b.Observed); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func scanBuilds(rows *sql.Rows) ([]Build, error) {
	defer rows.Close()
	out := []Build{}
	for rows.Next() {
		var b Build
		if err := rows.Scan(&b.ConnectionID, &b.Job, &b.Number, &b.Started, &b.Duration, &b.Status, &b.RawStatus, &b.Commit, &b.URL, &b.Observed); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) Running(id string) ([]Build, error) {
	rows, err := s.db.Query("SELECT * FROM builds WHERE connection_id=? AND status='RUNNING' ORDER BY started ASC LIMIT 100", id)
	if err != nil {
		return nil, err
	}
	return scanBuilds(rows)
}
func (s *Store) History(f HistoryFilter) (HistoryPage, error) {
	out := HistoryPage{Builds: []Build{}}
	where := []string{"1=1"}
	args := []any{}
	if f.ConnectionID != "" {
		where = append(where, "connection_id=?")
		args = append(args, f.ConnectionID)
	}
	if f.Job != "" {
		where = append(where, `job LIKE ? ESCAPE '\'`)
		q := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(f.Job)
		args = append(args, "%"+q+"%")
	}
	if f.Status != "" {
		where = append(where, "status=?")
		args = append(args, f.Status)
	}
	if f.Since > 0 {
		where = append(where, "started>=?")
		args = append(args, f.Since)
	}
	if f.Until > 0 {
		where = append(where, "started<=?")
		args = append(args, f.Until)
	}
	clause := strings.Join(where, " AND ")
	if err := s.db.QueryRow("SELECT count(*) FROM builds WHERE "+clause, args...).Scan(&out.Total); err != nil {
		return out, err
	}
	args = append(args, max(0, f.Offset))
	rows, err := s.db.Query("SELECT * FROM builds WHERE "+clause+" ORDER BY started DESC,connection_id,job,number DESC LIMIT 100 OFFSET ?", args...)
	if err != nil {
		return out, err
	}
	out.Builds, err = scanBuilds(rows)
	for _, p := range []string{s.Path, s.Path + "-wal"} {
		if stat, e := os.Stat(p); e == nil {
			out.Size += stat.Size()
		}
	}
	return out, err
}

func (s *Store) Preference(key string) (string, error) {
	var v string
	err := s.db.QueryRow("SELECT value FROM preferences WHERE key=?", key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}
func (s *Store) SavePreference(key, value string) error {
	if len(value) > 1024*1024 {
		return errors.New("Preference too large")
	}
	_, err := s.db.Exec("INSERT INTO preferences VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", key, value)
	return err
}

func (s *Store) Backup(path string) error {
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		return errors.New("Choose a new backup filename; existing files are not overwritten")
	}
	// VACUUM INTO produces a consistent standalone snapshot, including WAL contents.
	_, err := s.db.Exec("VACUUM INTO ?", path)
	return err
}
