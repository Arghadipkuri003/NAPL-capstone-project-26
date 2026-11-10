// Package store persists saved simulation runs in SQLite.
package store

import (
	"database/sql"
	"encoding/json"

	_ "modernc.org/sqlite"
)

type Run struct {
	ID        int64           `json:"id"`
	Name      string          `json:"name"`
	Algorithm string          `json:"algorithm"`
	Quantum   int             `json:"quantum"`
	AvgWT     float64         `json:"avg_wt"`
	AvgTAT    float64         `json:"avg_tat"`
	CreatedAt string          `json:"created_at"`
	Processes json.RawMessage `json:"processes,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
}

type Store struct{ db *sql.DB }

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS runs (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		name       TEXT NOT NULL,
		algorithm  TEXT NOT NULL,
		quantum    INTEGER NOT NULL DEFAULT 0,
		processes  TEXT NOT NULL,
		result     TEXT NOT NULL,
		avg_wt     REAL NOT NULL,
		avg_tat    REAL NOT NULL,
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`)
	if err != nil {
		return nil, err
	}
	return &Store{db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Save(r Run) (int64, error) {
	res, err := s.db.Exec(`INSERT INTO runs(name,algorithm,quantum,processes,result,avg_wt,avg_tat) VALUES(?,?,?,?,?,?,?)`,
		r.Name, r.Algorithm, r.Quantum, string(r.Processes), string(r.Result), r.AvgWT, r.AvgTAT)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) List() ([]Run, error) {
	rows, err := s.db.Query(`SELECT id,name,algorithm,quantum,avg_wt,avg_tat,created_at FROM runs ORDER BY id DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Run{}
	for rows.Next() {
		var r Run
		if err := rows.Scan(&r.ID, &r.Name, &r.Algorithm, &r.Quantum, &r.AvgWT, &r.AvgTAT, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) Get(id int64) (Run, error) {
	var r Run
	var p, res string
	err := s.db.QueryRow(`SELECT id,name,algorithm,quantum,processes,result,avg_wt,avg_tat,created_at FROM runs WHERE id=?`, id).
		Scan(&r.ID, &r.Name, &r.Algorithm, &r.Quantum, &p, &res, &r.AvgWT, &r.AvgTAT, &r.CreatedAt)
	r.Processes, r.Result = json.RawMessage(p), json.RawMessage(res)
	return r, err
}

func (s *Store) Delete(id int64) error {
	_, err := s.db.Exec(`DELETE FROM runs WHERE id=?`, id)
	return err
}
