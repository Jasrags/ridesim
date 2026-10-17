package engine

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// Event types. The vocabulary is shared by ride models and metrics.
const (
	EvRunStart   = "run_start"   // Info: run parameters
	EvQueueJoin  = "queue_join"  // Guest, N=party size, Info=lane
	EvBoard      = "board"       // Guest, Train, N=party size
	EvTrainEnter = "train_enter" // Train, Block
	EvHoldStart  = "hold_start"  // Train, Block, Info=reason
	EvHoldEnd    = "hold_end"    // Train, Block
	EvHoldYield  = "hold_yield"  // Train, Block, Info=rule that yielded to avoid a circular wait
	EvLoadDone   = "load_done"   // Train, N=riders aboard
	EvDispatch   = "dispatch"    // Train, Block (the station block left), N=riders
	EvUnload     = "unload"      // Train, N=riders off
	EvRideStop   = "ride_stop"   // Info=cause
	EvCascade    = "cascade"     // Train, Block: the train blocked past blocked_tolerance_s
	EvRideResume = "ride_resume" // Info=cause
	EvRunEnd     = "run_end"
)

// Event is one observable change. Field order is fixed, so JSON encoding is byte-stable.
type Event struct {
	T     Time   `json:"t"`
	Type  string `json:"type"`
	Train int    `json:"train,omitempty"` // 1-based; 0 means none
	Block string `json:"block,omitempty"`
	Guest int64  `json:"guest,omitempty"` // 1-based; 0 means none
	N     int    `json:"n,omitempty"`
	Info  string `json:"info,omitempty"`
}

// Log is the append-only event log.
type Log struct{ events []Event }

// Events returns the events in append order. Callers must not modify the slice.
func (l *Log) Events() []Event { return l.events }

func (l *Log) append(e Event) { l.events = append(l.events, e) }

// Write encodes the log as JSON lines.
func (l *Log) Write(w io.Writer) error {
	bw := bufio.NewWriter(w)
	enc := json.NewEncoder(bw)
	for i := range l.events {
		if err := enc.Encode(&l.events[i]); err != nil {
			return fmt.Errorf("encode event %d: %w", i, err)
		}
	}
	return bw.Flush()
}

// ReadLog decodes a JSON-lines log written by Write.
func ReadLog(r io.Reader) ([]Event, error) {
	var out []Event
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	for dec.More() {
		var e Event
		if err := dec.Decode(&e); err != nil {
			return nil, fmt.Errorf("decode event %d: %w", len(out), err)
		}
		out = append(out, e)
	}
	return out, nil
}
