// Package config holds the typed YAML schema for ride and park files, the versioned loader, and the
// invariant checks. Struct fields mirror the YAML exactly; decoding is strict (unknown fields fail).
package config

import "gopkg.in/yaml.v3"

// SchemaVersion is the only version the loader accepts.
const SchemaVersion = "1.0"

// Archetypes known to the loader.
const (
	ArchetypeMultiTrainCoaster = "multi_train_coaster"
	ArchetypeContinuousLoad    = "continuous_load"
)

// Dist is a normal distribution given as mean and standard deviation, in the unit the field name implies.
type Dist struct {
	Mean float64 `yaml:"mean"`
	SD   float64 `yaml:"sd"`
}

// ---------------------------------------------------------------------------------------------------
// Shared ride sections

type RideInfo struct {
	ID                  string         `yaml:"id"`
	Family              string         `yaml:"family"`
	Name                string         `yaml:"name"`
	Location            Location       `yaml:"location"`
	OtherVersions       []OtherVersion `yaml:"other_versions"`
	Variant             string         `yaml:"variant"`
	Archetype           string         `yaml:"archetype"`
	RideDurationS       float64        `yaml:"ride_duration_s"`
	TotalExperienceS    float64        `yaml:"total_experience_s"`
	HeightRestrictionIn float64        `yaml:"height_restriction_in"`
	ListedCapacityPerHr float64        `yaml:"listed_capacity_per_hr"`
	DataConflicts       []DataConflict `yaml:"data_conflicts"`
}

type Location struct {
	Resort string `yaml:"resort"`
	Park   string `yaml:"park"`
	Land   string `yaml:"land"`
}

type OtherVersion struct {
	Resort string `yaml:"resort"`
	Park   string `yaml:"park"`
	Status string `yaml:"status"`
	Note   string `yaml:"note"`
}

// DataConflict records disagreeing sources. Values and Chosen are heterogeneous in the data
// (numbers, lists, strings, null), so they stay as YAML nodes and are rendered in source order.
type DataConflict struct {
	Field  string    `yaml:"field"`
	Values yaml.Node `yaml:"values"`
	Chosen yaml.Node `yaml:"chosen"`
	Note   string    `yaml:"note"`
}

type Failures struct {
	CascadeStop  *CascadeStop  `yaml:"cascade_stop"`
	BriefStop    *RateStop     `yaml:"brief_stop"`
	ChainStop    *RateStop     `yaml:"chain_stop"`
	LongDowntime *LongDowntime `yaml:"long_downtime"`
	WeatherHold  string        `yaml:"weather_hold"`
}

type CascadeStop struct {
	Trigger           string  `yaml:"trigger"`
	BlockedToleranceS float64 `yaml:"blocked_tolerance_s"`
	AutoResume        bool    `yaml:"auto_resume"`
	OperatorResetS    Dist    `yaml:"operator_reset_s"`
	// Scope is which holds count: CascadeScopeCourseBrakes (default), CascadeScopeBrakesOnly or
	// CascadeScopeBrakesAndStations.
	Scope string `yaml:"scope"`
}

// Cascade scopes (decision 2026-10-17: course brakes; waiting at a station or at the brake feeding the
// station is the normal platform queue, not a blocked train).
const (
	CascadeScopeBrakesOnly        = "brakes_only"
	CascadeScopeBrakesAndStations = "brakes_and_stations"
	// CascadeScopeCourseBrakes counts brakes except the one feeding the station (normal platform queue).
	CascadeScopeCourseBrakes = "course_brakes"
)

type RateStop struct {
	RatePerHr float64 `yaml:"rate_per_hr"`
	DurationS Dist    `yaml:"duration_s"`
}

type LongDowntime struct {
	MTBFHr  float64 `yaml:"mtbf_hr"`
	MTTRMin Dist    `yaml:"mttr_min"`
}

type Preshow struct {
	Enabled              bool    `yaml:"enabled"`
	Rooms                int     `yaml:"rooms"`
	BatchSize            int     `yaml:"batch_size"`
	DurationS            Dist    `yaml:"duration_s"`
	TurnoverS            Dist    `yaml:"turnover_s"`
	FillRule             string  `yaml:"fill_rule"`
	MaxFillWaitS         float64 `yaml:"max_fill_wait_s"`
	ReleaseRule          string  `yaml:"release_rule"`
	PartyIntegrity       bool    `yaml:"party_integrity"`
	MergePoint           bool    `yaml:"merge_point"`
	BypassForSingleRider bool    `yaml:"bypass_for_single_rider"`
	LoadAreaCapacity     int     `yaml:"load_area_capacity"` // guests between preshow and boarding; 0 = unlimited
}

type Queues struct {
	PartySplitPolicy string      `yaml:"party_split_policy"`
	Standby          Standby     `yaml:"standby"`
	SingleRider      SingleRider `yaml:"single_rider"`
	SkipLine         SkipLine    `yaml:"skip_line"`
	PostedWait       PostedWait  `yaml:"posted_wait"`
}

type Standby struct {
	PhysicalCapacity int `yaml:"physical_capacity"`
}

type SingleRider struct {
	Enabled            bool `yaml:"enabled"`
	FillOnlyEmptySeats bool `yaml:"fill_only_empty_seats"`
}

type SkipLine struct {
	Enabled    bool   `yaml:"enabled"`
	MergePoint string `yaml:"merge_point"`
	Merge      Merge  `yaml:"merge"`
}

type Merge struct {
	Unit                  string       `yaml:"unit"`
	EmptyLaneRule         string       `yaml:"empty_lane_rule"`
	PhaseHysteresisGuests int          `yaml:"phase_hysteresis_guests"`
	Phases                []MergePhase `yaml:"phases"`
}

// MergePhase: WhenLLQueueGuestsBelow nil means "no upper bound" (the last phase).
type MergePhase struct {
	Name                   string `yaml:"name"`
	WhenLLQueueGuestsBelow *int   `yaml:"when_ll_queue_guests_below"`
	LLPerStandby           int    `yaml:"ll_per_standby"`
}

type PostedWait struct {
	Algorithm  string  `yaml:"algorithm"`
	PaddingPct float64 `yaml:"padding_pct"`
}

// ValidationTargets is the union of target keys used across ride files. They are outputs the
// simulator is checked against, never inputs.
type ValidationTargets struct {
	TheoreticalRidersPerHr              float64  `yaml:"theoretical_riders_per_hr"`
	ListedCapacityPerHr                 float64  `yaml:"listed_capacity_per_hr"`
	FiveTrainDispatchIntervalS          float64  `yaml:"five_train_dispatch_interval_s"`
	ExpectStackingAt5Trains             bool     `yaml:"expect_stacking_at_5_trains"`
	ExpectBrakeHoldsAt5Trains           []string `yaml:"expect_brake_holds_at_5_trains"`
	ExpectRareHoldsAtYetiBrake          bool     `yaml:"expect_rare_holds_at_yeti_brake"`
	ExpectGapAt4Trains                  bool     `yaml:"expect_gap_at_4_trains"`
	ImpliedDispatchIntervalS            float64  `yaml:"implied_dispatch_interval_s"`
	TheoreticalRidersPerHrAtTarget      float64  `yaml:"theoretical_riders_per_hr_at_target"`
	ExpectFourTrainsSufficient          bool     `yaml:"expect_four_trains_sufficient"`
	HardCeilingPerHr                    float64  `yaml:"hard_ceiling_per_hr"`
	ExpectNoStacking                    bool     `yaml:"expect_no_stacking"`
	ExpectCapacityLossFromAccessibility bool     `yaml:"expect_capacity_loss_from_accessibility_holds"`
	AvgStandbyWaitMin2026               float64  `yaml:"avg_standby_wait_min_2026"`
}

// ---------------------------------------------------------------------------------------------------
// multi_train_coaster

// Block kinds.
const (
	BlockStation = "station"
	BlockLift    = "lift"
	BlockLaunch  = "launch"
	BlockCourse  = "course"
	BlockBrake   = "brake"
)

type CoasterFile struct {
	SchemaVersion     string            `yaml:"schema_version"`
	Ride              RideInfo          `yaml:"ride"`
	Trains            Trains            `yaml:"trains"`
	Blocks            []Block           `yaml:"blocks"`
	Clearance         Clearance         `yaml:"clearance"`
	Station           Station           `yaml:"station"`
	Layout            *Layout           `yaml:"layout"`
	Barn              *Barn             `yaml:"barn"`
	GuestPath         []string          `yaml:"guest_path"`
	ExitPath          []string          `yaml:"exit_path"`
	Failures          Failures          `yaml:"failures"`
	Preshow           *Preshow          `yaml:"preshow"`
	Queues            Queues            `yaml:"queues"`
	ValidationTargets ValidationTargets `yaml:"validation_targets"`
	ConfigInvariants  []string          `yaml:"config_invariants"`
}

type Trains struct {
	FleetSize        int    `yaml:"fleet_size"`
	MaxInService     int    `yaml:"max_in_service"`
	DefaultInService int    `yaml:"default_in_service"`
	CarsPerTrain     int    `yaml:"cars_per_train"`
	SeatsPerTrain    int    `yaml:"seats_per_train"`
	SeatsPerRow      int    `yaml:"seats_per_row"` // parties take whole rows
	Layout           string `yaml:"layout"`
	Restraints       string `yaml:"restraints"`
}

type Block struct {
	ID             string   `yaml:"id"`
	Kind           string   `yaml:"kind"`
	TransitS       *Dist    `yaml:"transit_s"`
	Contains       []string `yaml:"contains"`
	Brake          *Brake   `yaml:"brake"`
	Launch         *Launch  `yaml:"launch"`
	HoldUntilClear bool     `yaml:"hold_until_clear"`
	Note           string   `yaml:"note"`
	Next           string   `yaml:"next"`
}

// Brake fields are pointers so the invariant can tell "missing" from "zero".
type Brake struct {
	HoldCapable   *bool  `yaml:"hold_capable"`
	StopPolicy    string `yaml:"stop_policy"`
	ReleaseDelayS *Dist  `yaml:"release_delay_s"`
}

// Brake stop policies.
const StopIfNextOccupied = "stop_if_next_occupied"

type Launch struct {
	AccelS      float64 `yaml:"accel_s"`
	TopSpeedMph float64 `yaml:"top_speed_mph"`
}

type Clearance struct {
	SwitchRealignS *Dist           `yaml:"switch_realign_s"`
	Rules          []ClearanceRule `yaml:"rules"`
}

// ClearanceRule: a train may leave block From only when every block in RequiresEmpty is empty and
// every condition holds.
type ClearanceRule struct {
	ID            string      `yaml:"id"`
	From          string      `yaml:"from"`
	RequiresEmpty []string    `yaml:"requires_empty"`
	Conditions    []Condition `yaml:"conditions"`
}

// Condition kinds.
const (
	// CondSwitchAligned: the named switch has finished realigning since the last train left the block
	// that contains it.
	CondSwitchAligned = "switch_aligned"
	// CondLoadClearToAdvance: the train entering UNLOAD must not be stranded behind a load train that
	// cannot advance. With separate UNLOAD/LOAD blocks this is already implied by UNLOAD being empty,
	// so it adds no gate of its own; it is kept so the data stays faithful to the fan description.
	CondLoadClearToAdvance = "load_clear_to_advance"
)

type Condition struct {
	Kind   string `yaml:"kind"`
	Switch string `yaml:"switch"`
}

type Station struct {
	SeparateLoadUnload      bool    `yaml:"separate_load_unload"`
	UnloadS                 Dist    `yaml:"unload_s"`
	LoadS                   Dist    `yaml:"load_s"`
	RestraintCheckS         Dist    `yaml:"restraint_check_s"`
	DispatchClearanceS      Dist    `yaml:"dispatch_clearance_s"`
	TargetDispatchIntervalS float64 `yaml:"target_dispatch_interval_s"`
	MinDispatchIntervalS    float64 `yaml:"min_dispatch_interval_s"`
	StaffAtStation          int     `yaml:"staff_at_station"`
	DispatchRule            string  `yaml:"dispatch_rule"`
	MaxWaitAfterReadyS      float64 `yaml:"max_wait_after_ready_s"`
}

// Dispatch rules.
const DispatchHoldForFullOrMaxWait = "hold_for_full_or_max_wait"

type Layout struct {
	Topology            string   `yaml:"topology"`
	StationSide         string   `yaml:"station_side"`
	Sequence            []string `yaml:"sequence"`
	LaunchStraight      string   `yaml:"launch_straight"`
	ReturnStraight      string   `yaml:"return_straight"`
	BrakePositionsOnMap string   `yaml:"brake_positions_on_map"`
}

// Barn: StorageBays nil means unknown (assume enough).
type Barn struct {
	Location    string `yaml:"location"`
	StorageBays *int   `yaml:"storage_bays"`
	SwapTimeS   Dist   `yaml:"swap_time_s"`
}

// ---------------------------------------------------------------------------------------------------
// continuous_load

// Route step kinds.
const (
	RouteLoad   = "load"
	RouteScene  = "scene"
	RouteUnload = "unload"
	RouteReturn = "return"
)

type ContinuousFile struct {
	SchemaVersion     string            `yaml:"schema_version"`
	Ride              RideInfo          `yaml:"ride"`
	Conveyor          Conveyor          `yaml:"conveyor"`
	Route             []RouteStep       `yaml:"route"`
	Vehicles          Vehicles          `yaml:"vehicles"`
	LoadZone          LoadZone          `yaml:"load_zone"`
	UnloadZone        UnloadZone        `yaml:"unload_zone"`
	LoadingShared     LoadingShared     `yaml:"loading_shared"`
	Failures          Failures          `yaml:"failures"`
	Preshow           *Preshow          `yaml:"preshow"`
	Queues            Queues            `yaml:"queues"`
	ValidationTargets ValidationTargets `yaml:"validation_targets"`
	ConfigInvariants  []string          `yaml:"config_invariants"`
}

type Conveyor struct {
	SpeedFps   float64 `yaml:"speed_fps"`
	Continuous bool    `yaml:"continuous"`
	Topology   string  `yaml:"topology"`
}

type RouteStep struct {
	ID    string  `yaml:"id"`
	Kind  string  `yaml:"kind"`
	MapPx float64 `yaml:"map_px"`
	EstS  float64 `yaml:"est_s"`
}

type Vehicles struct {
	Count           int     `yaml:"count"`
	SeatsNominal    int     `yaml:"seats_nominal"`
	SeatsMax        int     `yaml:"seats_max"`
	SpacingS        float64 `yaml:"spacing_s"`
	OccupancyTarget Dist    `yaml:"occupancy_target"`
}

type LoadZone struct {
	Type          string  `yaml:"type"`
	LoadWindowS   Dist    `yaml:"load_window_s"`
	Loaders       int     `yaml:"loaders"`
	BoardFailRate float64 `yaml:"board_fail_rate"`
}

type UnloadZone struct {
	Type          string `yaml:"type"`
	UnloadWindowS Dist   `yaml:"unload_window_s"`
	Unloaders     int    `yaml:"unloaders"`
}

type LoadingShared struct {
	AccessibilityFreezeZones []string          `yaml:"accessibility_freeze_zones"`
	AccessibilityHold        AccessibilityHold `yaml:"accessibility_hold"`
	PartyVehiclePolicy       string            `yaml:"party_vehicle_policy"`
}

type AccessibilityHold struct {
	RatePerHr float64 `yaml:"rate_per_hr"`
	DurationS Dist    `yaml:"duration_s"`
}

// Party vehicle policies.
const ContiguousVehicles = "contiguous_vehicles"

// ---------------------------------------------------------------------------------------------------
// park

// Park node kinds and walking modes.
const (
	NodeEntrance   = "entrance"
	NodePlaza      = "plaza"
	NodeRide       = "ride"
	NodeRideSimple = "ride_simple"
	NodeShow       = "show"
	NodeDining     = "dining"
	NodeExit       = "exit"

	WalkingFixedDelay = "fixed_delay"
	WalkingCongestion = "congestion"
)

type ParkFile struct {
	SchemaVersion      string             `yaml:"schema_version"`
	Park               ParkInfo           `yaml:"park"`
	Nodes              []ParkNode         `yaml:"nodes"`
	Edges              []ParkEdge         `yaml:"edges"`
	Congestion         Congestion         `yaml:"congestion"`
	Guests             Guests             `yaml:"guests"`
	Decision           Decision           `yaml:"decision"`
	CalibrationTargets CalibrationTargets `yaml:"calibration_targets"`
}

type ParkInfo struct {
	ID          string `yaml:"id"`
	OpenHour    int    `yaml:"open_hour"`
	CloseHour   int    `yaml:"close_hour"`
	WalkingMode string `yaml:"walking_mode"`
}

// ParkNode is the union of node kinds; which fields are required depends on Kind.
type ParkNode struct {
	ID                string   `yaml:"id"`
	Kind              string   `yaml:"kind"`
	RideID            string   `yaml:"ride_id"`
	Appeal            float64  `yaml:"appeal"`
	Thrill            float64  `yaml:"thrill"`
	ComfortableGuests int      `yaml:"comfortable_guests"`
	CapacityPerHr     float64  `yaml:"capacity_per_hr"`
	DurationS         float64  `yaml:"duration_s"`
	CapacityGuests    int      `yaml:"capacity_guests"`
	DurationMin       float64  `yaml:"duration_min"`
	Schedule          []string `yaml:"schedule"`
	ExitEdge          string   `yaml:"exit_edge"`
	Seats             int      `yaml:"seats"`
	ServiceTimeMin    *Dist    `yaml:"service_time_min"`
}

type ParkEdge struct {
	ID       string  `yaml:"id"`
	From     string  `yaml:"from"`
	To       string  `yaml:"to"`
	LengthFt float64 `yaml:"length_ft"`
	WidthFt  float64 `yaml:"width_ft"`
}

type Congestion struct {
	FreeSpeedFps              float64   `yaml:"free_speed_fps"`
	ComfortableDensityPerSqft float64   `yaml:"comfortable_density_per_sqft"`
	Alpha                     float64   `yaml:"alpha"`
	Beta                      float64   `yaml:"beta"`
	JamDensityPerSqft         float64   `yaml:"jam_density_per_sqft"`
	Spillback                 Spillback `yaml:"spillback"`
}

type Spillback struct {
	Enabled                     bool     `yaml:"enabled"`
	WidthConsumedFtPer100Guests float64  `yaml:"width_consumed_ft_per_100_guests"`
	AppliesTo                   []string `yaml:"applies_to"`
}

type Guests struct {
	DailyAttendance         DailyAttendance    `yaml:"daily_attendance"`
	ArrivalCurveShareByHour map[string]float64 `yaml:"arrival_curve_share_by_hour"`
	PartySize               map[int]float64    `yaml:"party_size"`
	StayHours               Dist               `yaml:"stay_hours"`
	PatienceMin             Dist               `yaml:"patience_min"`
	ThrillTolerance         Dist               `yaml:"thrill_tolerance"`
	LLPassesPerGuest        Dist               `yaml:"ll_passes_per_guest"`
}

type DailyAttendance struct {
	Low    int `yaml:"low"`
	Medium int `yaml:"medium"`
	High   int `yaml:"high"`
}

type Decision struct {
	Method             string   `yaml:"method"`
	Temperature        float64  `yaml:"temperature"`
	ChoiceSet          string   `yaml:"choice_set"`
	K                  int      `yaml:"k"`
	Utility            Utility  `yaml:"utility"`
	PostedWaitRefreshS float64  `yaml:"posted_wait_refresh_s"`
	FatiguePerHour     float64  `yaml:"fatigue_per_hour"`
	MealWindows        []string `yaml:"meal_windows"`
}

type Utility struct {
	Appeal              float64 `yaml:"appeal"`
	ThrillMatch         float64 `yaml:"thrill_match"`
	WaitPenaltyPerMin   float64 `yaml:"wait_penalty_per_min"`
	WalkPenaltyPerMin   float64 `yaml:"walk_penalty_per_min"`
	RepeatPenalty       float64 `yaml:"repeat_penalty"`
	LLReturnWindowBonus float64 `yaml:"ll_return_window_bonus"`
}

type CalibrationTargets struct {
	EverestAvgStandbyMin         float64 `yaml:"everest_avg_standby_min"`
	EverestPeakStandbyMin        float64 `yaml:"everest_peak_standby_min"`
	HauntedMansionAvgStandbyMin  float64 `yaml:"haunted_mansion_avg_standby_min"`
	HauntedMansionPeakStandbyMin float64 `yaml:"haunted_mansion_peak_standby_min"`
	ExpectWaitSpikeAfterTheater  bool    `yaml:"expect_wait_spike_after_theater_shows"`
}
