package scheduler

import (
	"strings"
	"testing"
)

func cts(r Result) map[string]int {
	m := map[string]int{}
	for _, x := range r.Metrics {
		m[x.PID] = x.Completion
	}
	return m
}

func TestFCFSMatchesProposalExample(t *testing.T) {
	ps := []Process{{"P1", 0, 5, 1}, {"P2", 1, 3, 1}, {"P3", 2, 2, 1}}
	r, err := Run("fcfs", ps, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := [][4]int{{5, 5, 0, 0}, {8, 7, 4, 4}, {10, 8, 6, 6}} // CT TAT WT RT
	for i, w := range want {
		m := r.Metrics[i]
		if [4]int{m.Completion, m.Turnaround, m.Waiting, m.Response} != w {
			t.Errorf("%s got %+v want %v", m.PID, m, w)
		}
	}
}

func TestRoundRobin(t *testing.T) {
	ps := []Process{{"P1", 0, 5, 0}, {"P2", 1, 3, 0}, {"P3", 2, 1, 0}, {"P4", 3, 2, 0}}
	r, _ := Run("rr", ps, 2)
	got := cts(r)
	for pid, want := range map[string]int{"P1": 11, "P2": 10, "P3": 5, "P4": 9} {
		if got[pid] != want {
			t.Errorf("%s CT=%d want %d", pid, got[pid], want)
		}
	}
}

func TestSRTFPreempts(t *testing.T) {
	ps := []Process{{"P1", 0, 8, 0}, {"P2", 1, 4, 0}, {"P3", 2, 2, 0}}
	r, _ := Run("srtf", ps, 0)
	got := cts(r)
	if got["P1"] != 14 || got["P2"] != 7 || got["P3"] != 4 {
		t.Errorf("unexpected completions %v", got)
	}
}

func TestSJFNonPreemptive(t *testing.T) {
	ps := []Process{{"A", 0, 7, 0}, {"B", 2, 4, 0}, {"C", 4, 1, 0}, {"D", 5, 4, 0}}
	r, _ := Run("sjf", ps, 0)
	got := cts(r)
	if got["A"] != 7 || got["C"] != 8 || got["B"] != 12 || got["D"] != 16 {
		t.Errorf("unexpected completions %v", got)
	}
}

func TestPriorityLowerNumberWins(t *testing.T) {
	ps := []Process{{"A", 0, 3, 2}, {"B", 0, 3, 1}}
	r, _ := Run("priority", ps, 0)
	if r.Segments[0].PID != "B" {
		t.Errorf("expected B first, got %v", r.Segments)
	}
}

func TestIdleGap(t *testing.T) {
	for _, algo := range Order {
		r, err := Run(algo, []Process{{"P1", 3, 2, 1}}, 2)
		if err != nil || r.Segments[0].PID != Idle || r.Segments[0].End != 3 || r.Metrics[0].Completion != 5 {
			t.Errorf("%s: bad idle handling %+v %v", algo, r.Segments, err)
		}
	}
}

func TestInvariantsAllAlgorithms(t *testing.T) {
	ps := []Process{{"A", 0, 6, 3}, {"B", 1, 4, 1}, {"C", 2, 8, 2}, {"D", 20, 3, 1}}
	for _, algo := range Order {
		r, err := Run(algo, ps, 3)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range r.Metrics {
			if m.Turnaround != m.Completion-m.Arrival || m.Waiting != m.Turnaround-m.Burst || m.Waiting < 0 || m.Response < 0 {
				t.Errorf("%s %s: bad metrics %+v", algo, m.PID, m)
			}
		}
	}
}

func TestValidation(t *testing.T) {
	bad := []struct {
		ps []Process
		a  string
		q  int
	}{
		{nil, "fcfs", 0},
		{[]Process{{"P1", -1, 2, 0}}, "fcfs", 0},
		{[]Process{{"P1", 0, 0, 0}}, "fcfs", 0},
		{[]Process{{"P1", 0, 1, 0}, {"P1", 0, 1, 0}}, "fcfs", 0},
		{[]Process{{"P1", 0, 1, 0}}, "rr", 0},
		{[]Process{{"P1", 0, 1, 0}}, "nope", 0},
	}
	for i, b := range bad {
		if _, err := Run(b.a, b.ps, b.q); err == nil {
			t.Errorf("case %d should fail", i)
		}
	}
}

func TestCSV(t *testing.T) {
	r, _ := Run("fcfs", []Process{{"P1", 0, 2, 0}}, 0)
	var sb strings.Builder
	if err := WriteCSV(&sb, r); err != nil || !strings.Contains(sb.String(), "AVG") {
		t.Errorf("csv: %v %q", err, sb.String())
	}
}
