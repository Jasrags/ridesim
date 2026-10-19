package config

import "testing"

const (
	fxGate     = "  - { id: gate, kind: entrance, x: 0, y: 300 }"
	fxTwoGates = "  - { id: gate, kind: entrance, x: 0, y: 300, arrival_share: 0.7, exit: exit }\n" +
		"  - { id: gate2, kind: entrance, x: 380, y: 300, arrival_share: 0.3, exit: exit2 }\n" +
		"  - { id: exit2, kind: exit, x: 380, y: 300 }"
)

// twoGates patches the fixture park to two gates (new nodes joined to the hub).
func twoGates(t *testing.T, gates string) *File {
	t.Helper()
	f := patched(t, fxPark, "", fxGate, gates)
	f.Park.Edges = append(f.Park.Edges, ParkEdge{ID: "hub_gate2", From: "hub", To: "gate2", LengthFt: 100, WidthFt: 10},
		ParkEdge{ID: "hub_exit2", From: "hub", To: "exit2", LengthFt: 100, WidthFt: 10})
	return f
}

func TestTwoGatesAndNodeHoursValid(t *testing.T) {
	f := twoGates(t, fxTwoGates)
	for _, r := range CheckFile(f) {
		if r.Status != StatusPass {
			t.Errorf("two gates: %s: %s", r.CheckID, r.Detail)
		}
	}
	h := patched(t, fxPark, "", "capacity_per_hr: 100,", `capacity_per_hr: 100, hours: "11:00-17:00",`)
	if r := resultFor(t, CheckFile(h), "park.node_hours"); r.Status != StatusPass {
		t.Errorf("hours: %s", r.Detail)
	}
}

func TestGateAndHoursChecks(t *testing.T) {
	cases := []struct {
		name, check string
		f           func(t *testing.T) *File
	}{
		{"share on a single gate", "park.gates", func(t *testing.T) *File {
			return patched(t, fxPark, "", fxGate, "  - { id: gate, kind: entrance, x: 0, y: 300, arrival_share: 1 }")
		}},
		{"shares do not sum to 1", "park.gates", func(t *testing.T) *File {
			return twoGates(t, fxTwoGates[:0]+"  - { id: gate, kind: entrance, x: 0, y: 300, arrival_share: 0.5, exit: exit }\n"+
				"  - { id: gate2, kind: entrance, x: 380, y: 300, arrival_share: 0.3, exit: exit2 }\n  - { id: exit2, kind: exit, x: 380, y: 300 }")
		}},
		{"exit not an exit", "park.gates", func(t *testing.T) *File {
			return twoGates(t, "  - { id: gate, kind: entrance, x: 0, y: 300, arrival_share: 0.7, exit: hub }\n"+
				"  - { id: gate2, kind: entrance, x: 380, y: 300, arrival_share: 0.3, exit: exit2 }\n  - { id: exit2, kind: exit, x: 380, y: 300 }")
		}},
		{"unused exit", "park.gates", func(t *testing.T) *File {
			return twoGates(t, "  - { id: gate, kind: entrance, x: 0, y: 300, arrival_share: 0.7, exit: exit2 }\n"+
				"  - { id: gate2, kind: entrance, x: 380, y: 300, arrival_share: 0.3, exit: exit2 }\n  - { id: exit2, kind: exit, x: 380, y: 300 }")
		}},
		{"hours before opening", "park.node_hours", func(t *testing.T) *File {
			return patched(t, fxPark, "", "capacity_per_hr: 100,", `capacity_per_hr: 100, hours: "08:00-12:00",`)
		}},
		{"hours backwards", "park.node_hours", func(t *testing.T) *File {
			return patched(t, fxPark, "", "capacity_per_hr: 100,", `capacity_per_hr: 100, hours: "12:00-11:00",`)
		}},
		{"hours on a plaza", "park.node_hours", func(t *testing.T) *File {
			return patched(t, fxPark, "", "comfortable_guests: 100,", `comfortable_guests: 100, hours: "11:00-12:00",`)
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if r := resultFor(t, CheckFile(c.f(t)), c.check); r.Status != StatusFail || r.Severity != SevError {
				t.Fatalf("got %v/%v (%s), want error FAIL", r.Severity, r.Status, r.Detail)
			}
		})
	}
}
