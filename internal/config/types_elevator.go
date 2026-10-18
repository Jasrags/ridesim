package config

// ---------------------------------------------------------------------------------------------------
// elevator_drop (Tower of Terror, M10)

// ArchetypeElevatorDrop is a drop-shaft ride with self-propelled cabs (Tower of Terror): the building
// has mirrored sides, each with show shafts that lift a cab from the basement to load and on through
// the show scenes, one drop shaft, and unload doors in the basement.
const ArchetypeElevatorDrop = "elevator_drop"

// FailSideDown takes one side out of service for a while; the other side keeps running.
const FailSideDown = "side_down"

type ElevatorFile struct {
	SchemaVersion     string            `yaml:"schema_version"`
	Ride              RideInfo          `yaml:"ride"`
	Vehicles          ElevatorVehicles  `yaml:"vehicles"`
	Sides             ElevatorSides     `yaml:"sides"`
	Cycle             ElevatorCycle     `yaml:"cycle"`
	Failures          Failures          `yaml:"failures"`
	Preshow           *Preshow          `yaml:"preshow"`
	Queues            Queues            `yaml:"queues"`
	ValidationTargets ValidationTargets `yaml:"validation_targets"`
	ConfigInvariants  []string          `yaml:"config_invariants"`
}

type ElevatorVehicles struct {
	FleetSize       int    `yaml:"fleet_size"`
	PerSide         int    `yaml:"per_side"` // cabs in service on each side
	SeatsPerVehicle int    `yaml:"seats_per_vehicle"`
	SeatsPerRow     int    `yaml:"seats_per_row"` // parties take whole rows of this many seats
	Layout          string `yaml:"layout"`
	Restraints      string `yaml:"restraints"`
}

type ElevatorSides struct {
	Count       int      `yaml:"count"`
	Names       []string `yaml:"names"`        // one per side, for the log
	ShowShafts  int      `yaml:"show_shafts"`  // per side
	DropShafts  int      `yaml:"drop_shafts"`  // per side
	UnloadDoors int      `yaml:"unload_doors"` // per side
}

// ElevatorCycle is one cab's lap, in order. The cab holds a show shaft from to_show_shaft_s until it
// moves into the drop shaft, holds the drop shaft through drop_s until an unload door is free, and holds
// the door from to_unload_s through unload_s until a show shaft is free.
type ElevatorCycle struct {
	UnloadS            Dist    `yaml:"unload_s"`        // at the basement unload door
	ToShowShaftS       Dist    `yaml:"to_show_shaft_s"` // back out, pivot, drive into a show shaft
	LiftToLoadS        Dist    `yaml:"lift_to_load_s"`  // show shaft up one floor to load
	LoadS              Dist    `yaml:"load_s"`          // boarding window
	RestraintCheckS    Dist    `yaml:"restraint_check_s"`
	DispatchClearanceS Dist    `yaml:"dispatch_clearance_s"`
	MaxWaitAfterReadyS float64 `yaml:"max_wait_after_ready_s"` // hold an unfilled cab this long after load_s
	ShowS              Dist    `yaml:"show_s"`                 // lift, hallway, Fifth Dimension, up to the drop shaft door
	DropS              Dist    `yaml:"drop_s"`                 // into the drop shaft, drop sequence, down and out
	ToUnloadS          Dist    `yaml:"to_unload_s"`            // drop shaft to an unload door
}

// SideInterval is the theoretical time between dispatches on one side: the slowest of the drop shafts,
// the show shafts and the cab fleet, each divided by how many work in parallel.
func (c *ElevatorFile) SideInterval() float64 {
	y, s := c.Cycle, c.Sides
	station := y.LoadS.Mean + y.RestraintCheckS.Mean + y.DispatchClearanceS.Mean
	show := y.ToShowShaftS.Mean + y.LiftToLoadS.Mean + station + y.ShowS.Mean
	drop := y.DropS.Mean
	lap := y.UnloadS.Mean + show + drop + y.ToUnloadS.Mean
	iv := 0.0
	if s.ShowShafts > 0 {
		iv = max(iv, show/float64(s.ShowShafts))
	}
	if s.DropShafts > 0 {
		iv = max(iv, drop/float64(s.DropShafts))
	}
	if s.UnloadDoors > 0 {
		iv = max(iv, (y.ToUnloadS.Mean+y.UnloadS.Mean)/float64(s.UnloadDoors))
	}
	if c.Vehicles.PerSide > 0 {
		iv = max(iv, lap/float64(c.Vehicles.PerSide))
	}
	return iv
}
