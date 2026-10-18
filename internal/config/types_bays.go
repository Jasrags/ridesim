package config

// ---------------------------------------------------------------------------------------------------
// simulator_bays (Star Tours, M13)

// ArchetypeSimulatorBays is a ride of identical simulator bays that each run their own cycle (Jason,
// 2026-10-18): a bay boards a group at its gate, checks restraints, plays the safety video and the
// flight, lets the riders out, and boards again. No clock ties the bays together.
const ArchetypeSimulatorBays = "simulator_bays"

type BaysFile struct {
	SchemaVersion     string            `yaml:"schema_version"`
	Ride              RideInfo          `yaml:"ride"`
	Bays              Bays              `yaml:"bays"`
	Cycle             BayCycle          `yaml:"cycle"`
	Failures          Failures          `yaml:"failures"` // brief_stop, long_downtime, side_down (one bay)
	Preshow           *Preshow          `yaml:"preshow"`
	Queues            Queues            `yaml:"queues"`
	ValidationTargets ValidationTargets `yaml:"validation_targets"`
	ConfigInvariants  []string          `yaml:"config_invariants"`
}

type Bays struct {
	Count          int    `yaml:"count"`
	SeatsPerBay    int    `yaml:"seats_per_bay"`
	SeatsPerRow    int    `yaml:"seats_per_row"`   // row rule when boarding_groups is empty
	BoardingGroups []int  `yaml:"boarding_groups"` // rows cast assign parties to, front to back
	Layout         string `yaml:"layout"`
	Restraints     string `yaml:"restraints"`
}

// BayCycle is one bay's cycle, in order: the previous riders exit and the rows clear, the next group
// boards, restraints are checked, then the safety video and the flight.
type BayCycle struct {
	ExitS              Dist    `yaml:"exit_s"`
	LoadS              Dist    `yaml:"load_s"`
	MaxWaitAfterReadyS float64 `yaml:"max_wait_after_ready_s"`
	CalloutParties     int     `yaml:"callout_parties"` // boarding groups only: how far back cast call a party that fits
	RestraintCheckS    Dist    `yaml:"restraint_check_s"`
	SafetyVideoS       Dist    `yaml:"safety_video_s"`
	FlightS            Dist    `yaml:"flight_s"`
}

// CycleS is the mean time between one bay's launches when it fills within its boarding window.
func (c *BaysFile) CycleS() float64 {
	y := c.Cycle
	return y.ExitS.Mean + y.LoadS.Mean + y.RestraintCheckS.Mean + y.SafetyVideoS.Mean + y.FlightS.Mean
}
