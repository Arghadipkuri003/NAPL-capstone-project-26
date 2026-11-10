// Package scheduler implements the CPU scheduling algorithms, metric
// calculation and CSV export. It has no I/O dependencies besides io.Writer.
package scheduler

import (
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
)

const Idle = "IDLE"

type Process struct {
	PID      string `json:"pid"`
	Arrival  int    `json:"arrival"`
	Burst    int    `json:"burst"`
	Priority int    `json:"priority"` // lower number = higher priority
}

type Segment struct {
	PID   string `json:"pid"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

type Metric struct {
	PID        string `json:"pid"`
	Arrival    int    `json:"arrival"`
	Burst      int    `json:"burst"`
	Priority   int    `json:"priority"`
	Completion int    `json:"ct"`
	Turnaround int    `json:"tat"`
	Waiting    int    `json:"wt"`
	Response   int    `json:"rt"`
}

type Result struct {
	Algorithm  string    `json:"algorithm"`
	Label      string    `json:"label"`
	Quantum    int       `json:"quantum,omitempty"`
	Segments   []Segment `json:"segments"`
	Metrics    []Metric  `json:"metrics"`
	AvgTAT     float64   `json:"avg_tat"`
	AvgWT      float64   `json:"avg_wt"`
	AvgRT      float64   `json:"avg_rt"`
	CPUUtil    float64   `json:"cpu_util"`   // percent
	Throughput float64   `json:"throughput"` // processes per time unit
	Makespan   int       `json:"makespan"`
}

// Order is the display order of algorithms; Labels maps key -> human name.
var Order = []string{"fcfs", "sjf", "srtf", "priority", "priority_p", "rr"}
var Labels = map[string]string{
	"fcfs": "FCFS", "sjf": "SJF (non-preemptive)", "srtf": "SRTF (preemptive SJF)",
	"priority": "Priority (non-preemptive)", "priority_p": "Priority (preemptive)", "rr": "Round Robin",
}

func Validate(ps []Process, algo string, quantum int) error {
	if _, ok := Labels[algo]; !ok {
		return fmt.Errorf("unknown algorithm %q", algo)
	}
	if len(ps) == 0 {
		return fmt.Errorf("add at least one process")
	}
	if len(ps) > 200 {
		return fmt.Errorf("too many processes (max 200)")
	}
	seen := map[string]bool{}
	for _, p := range ps {
		switch {
		case p.PID == "":
			return fmt.Errorf("every process needs a PID")
		case seen[p.PID]:
			return fmt.Errorf("duplicate PID %q", p.PID)
		case p.Arrival < 0:
			return fmt.Errorf("%s: arrival time cannot be negative", p.PID)
		case p.Burst <= 0:
			return fmt.Errorf("%s: burst time must be greater than 0", p.PID)
		case p.Burst > 100000 || p.Arrival > 100000:
			return fmt.Errorf("%s: values too large", p.PID)
		}
		seen[p.PID] = true
	}
	if algo == "rr" && quantum <= 0 {
		return fmt.Errorf("Round Robin needs a time quantum greater than 0")
	}
	return nil
}

func Run(algo string, in []Process, quantum int) (Result, error) {
	if err := Validate(in, algo, quantum); err != nil {
		return Result{}, err
	}
	ps := append([]Process(nil), in...)
	var segs []Segment
	if algo == "rr" {
		segs = roundRobin(ps, quantum)
	} else {
		segs = keyed(algo, ps)
	}
	res := build(algo, ps, segs)
	if algo == "rr" {
		res.Quantum = quantum
	}
	return res, nil
}

func add(segs []Segment, pid string, s, e int) []Segment {
	if n := len(segs); n > 0 && segs[n-1].PID == pid && segs[n-1].End == s {
		segs[n-1].End = e
		return segs
	}
	return append(segs, Segment{pid, s, e})
}

// keyed handles FCFS, SJF, SRTF and both Priority variants. Ties are broken
// by arrival time, then input order.
func keyed(algo string, ps []Process) []Segment {
	n := len(ps)
	rem := make([]int, n)
	for i, p := range ps {
		rem[i] = p.Burst
	}
	preempt := algo == "srtf" || algo == "priority_p"
	better := func(a, b int) bool {
		var ka, kb int
		switch algo {
		case "sjf", "srtf":
			ka, kb = rem[a], rem[b]
		case "priority", "priority_p":
			ka, kb = ps[a].Priority, ps[b].Priority
		}
		if ka != kb {
			return ka < kb
		}
		if ps[a].Arrival != ps[b].Arrival {
			return ps[a].Arrival < ps[b].Arrival
		}
		return a < b
	}
	var segs []Segment
	t, done := 0, 0
	for done < n {
		best, nextArr := -1, math.MaxInt
		for i := range ps {
			if rem[i] == 0 {
				continue
			}
			if ps[i].Arrival <= t {
				if best < 0 || better(i, best) {
					best = i
				}
			} else if ps[i].Arrival < nextArr {
				nextArr = ps[i].Arrival
			}
		}
		if best < 0 { // CPU idle until next arrival
			segs = add(segs, Idle, t, nextArr)
			t = nextArr
			continue
		}
		run := rem[best]
		if preempt {
			run = 1
		}
		segs = add(segs, ps[best].PID, t, t+run)
		t += run
		rem[best] -= run
		if rem[best] == 0 {
			done++
		}
	}
	return segs
}

func roundRobin(ps []Process, q int) []Segment {
	n := len(ps)
	rem := make([]int, n)
	order := make([]int, n)
	for i, p := range ps {
		rem[i], order[i] = p.Burst, i
	}
	sort.SliceStable(order, func(a, b int) bool { return ps[order[a]].Arrival < ps[order[b]].Arrival })
	var queue []int
	var segs []Segment
	next, t, done := 0, 0, 0
	admit := func(upTo int) {
		for next < n && ps[order[next]].Arrival <= upTo {
			queue = append(queue, order[next])
			next++
		}
	}
	for done < n {
		admit(t)
		if len(queue) == 0 {
			at := ps[order[next]].Arrival
			segs = add(segs, Idle, t, at)
			t = at
			continue
		}
		i := queue[0]
		queue = queue[1:]
		run := min(q, rem[i])
		segs = add(segs, ps[i].PID, t, t+run)
		t += run
		rem[i] -= run
		admit(t) // new arrivals queue before the preempted process
		if rem[i] > 0 {
			queue = append(queue, i)
		} else {
			done++
		}
	}
	return segs
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

func build(algo string, ps []Process, segs []Segment) Result {
	first, last := map[string]int{}, map[string]int{}
	busy, end := 0, 0
	for _, s := range segs {
		end = s.End
		if s.PID == Idle {
			continue
		}
		if _, ok := first[s.PID]; !ok {
			first[s.PID] = s.Start
		}
		last[s.PID] = s.End
		busy += s.End - s.Start
	}
	r := Result{Algorithm: algo, Label: Labels[algo], Segments: segs, Makespan: end}
	var sT, sW, sR float64
	for _, p := range ps {
		ct := last[p.PID]
		tat := ct - p.Arrival
		m := Metric{p.PID, p.Arrival, p.Burst, p.Priority, ct, tat, tat - p.Burst, first[p.PID] - p.Arrival}
		r.Metrics = append(r.Metrics, m)
		sT, sW, sR = sT+float64(m.Turnaround), sW+float64(m.Waiting), sR+float64(m.Response)
	}
	n := float64(len(ps))
	r.AvgTAT, r.AvgWT, r.AvgRT = round2(sT/n), round2(sW/n), round2(sR/n)
	if end > 0 {
		r.CPUUtil = round2(float64(busy) / float64(end) * 100)
		r.Throughput = round2(n / float64(end))
	}
	return r
}

// WriteCSV writes the metrics table followed by the averages row.
func WriteCSV(w io.Writer, r Result) error {
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"PID", "AT", "BT", "Priority", "CT", "TAT", "WT", "RT"})
	for _, m := range r.Metrics {
		_ = cw.Write([]string{m.PID, strconv.Itoa(m.Arrival), strconv.Itoa(m.Burst), strconv.Itoa(m.Priority),
			strconv.Itoa(m.Completion), strconv.Itoa(m.Turnaround), strconv.Itoa(m.Waiting), strconv.Itoa(m.Response)})
	}
	f := func(v float64) string { return strconv.FormatFloat(v, 'f', 2, 64) }
	_ = cw.Write([]string{"AVG", "", "", "", "", f(r.AvgTAT), f(r.AvgWT), f(r.AvgRT)})
	cw.Flush()
	return cw.Error()
}
