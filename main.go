// CPU Scheduling Simulator: Go backend + embedded web GUI + SQLite history.
package main

import (
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strconv"

	"cpusim/scheduler"
	"cpusim/store"
)

//go:embed web
var webFS embed.FS

type server struct{ db *store.Store }

type request struct {
	Name      string              `json:"name"`
	Algorithm string              `json:"algorithm"`
	Quantum   int                 `json:"quantum"`
	Processes []scheduler.Process `json:"processes"`
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

func decode(w http.ResponseWriter, r *http.Request) (request, bool) {
	var req request
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		fail(w, http.StatusBadRequest, fmt.Errorf("invalid JSON: %w", err))
		return req, false
	}
	return req, true
}

func (s *server) simulate(w http.ResponseWriter, r *http.Request) {
	req, ok := decode(w, r)
	if !ok {
		return
	}
	res, err := scheduler.Run(req.Algorithm, req.Processes, req.Quantum)
	if err != nil {
		fail(w, http.StatusUnprocessableEntity, err)
		return
	}
	writeJSON(w, 200, res)
}

func (s *server) compare(w http.ResponseWriter, r *http.Request) {
	req, ok := decode(w, r)
	if !ok {
		return
	}
	q := req.Quantum
	if q <= 0 {
		q = 2
	}
	out := []scheduler.Result{}
	for _, a := range scheduler.Order {
		res, err := scheduler.Run(a, req.Processes, q)
		if err != nil {
			fail(w, http.StatusUnprocessableEntity, err)
			return
		}
		out = append(out, res)
	}
	writeJSON(w, 200, out)
}

func (s *server) csvExport(w http.ResponseWriter, r *http.Request) {
	req, ok := decode(w, r)
	if !ok {
		return
	}
	res, err := scheduler.Run(req.Algorithm, req.Processes, req.Quantum)
	if err != nil {
		fail(w, http.StatusUnprocessableEntity, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="schedule-`+req.Algorithm+`.csv"`)
	_ = scheduler.WriteCSV(w, res)
}

func (s *server) save(w http.ResponseWriter, r *http.Request) {
	req, ok := decode(w, r)
	if !ok {
		return
	}
	res, err := scheduler.Run(req.Algorithm, req.Processes, req.Quantum) // re-run: never trust client results
	if err != nil {
		fail(w, http.StatusUnprocessableEntity, err)
		return
	}
	if req.Name == "" {
		req.Name = res.Label
	}
	p, _ := json.Marshal(req.Processes)
	rb, _ := json.Marshal(res)
	id, err := s.db.Save(store.Run{Name: req.Name, Algorithm: req.Algorithm, Quantum: req.Quantum,
		Processes: p, Result: rb, AvgWT: res.AvgWT, AvgTAT: res.AvgTAT})
	if err != nil {
		fail(w, 500, err)
		return
	}
	writeJSON(w, 201, map[string]int64{"id": id})
}

func (s *server) list(w http.ResponseWriter, r *http.Request) {
	runs, err := s.db.List()
	if err != nil {
		fail(w, 500, err)
		return
	}
	writeJSON(w, 200, runs)
}

func (s *server) get(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	run, err := s.db.Get(id)
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, 404, errors.New("run not found"))
		return
	} else if err != nil {
		fail(w, 500, err)
		return
	}
	writeJSON(w, 200, run)
}

func (s *server) del(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err := s.db.Delete(id); err != nil {
		fail(w, 500, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "listen address")
	dbPath := flag.String("db", "simulator.db", "SQLite database file")
	flag.Parse()

	db, err := store.Open(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	s := &server{db}

	sub, _ := fs.Sub(webFS, "web")
	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServer(http.FS(sub)))
	mux.HandleFunc("POST /api/simulate", s.simulate)
	mux.HandleFunc("POST /api/compare", s.compare)
	mux.HandleFunc("POST /api/csv", s.csvExport)
	mux.HandleFunc("POST /api/runs", s.save)
	mux.HandleFunc("GET /api/runs", s.list)
	mux.HandleFunc("GET /api/runs/{id}", s.get)
	mux.HandleFunc("DELETE /api/runs/{id}", s.del)

	log.Printf("CPU Scheduling Simulator running at http://%s", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}
