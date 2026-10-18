package park

import (
	"slices"
	"testing"

	"github.com/jasrags/ridesim/internal/config"
)

// Each ride type gets the scenario's failure kinds it knows. Continuous rides call their brief stop
// chain_stop, so a scenario asking for brief_stop gives them chain_stop too (once, even if listed).
func TestRideFailures(t *testing.T) {
	dhs := []string{config.FailBriefStop, config.FailLongDowntime, config.FailCascadeStop, config.FailOpeningDelay}
	synthetic := []string{config.FailBriefStop, config.FailLongDowntime, config.FailCascadeStop, config.FailChainStop}
	cases := []struct {
		name     string
		kind     config.Kind
		scenario []string
		want     []string
	}{
		{"coaster", config.KindCoaster, dhs, []string{config.FailBriefStop, config.FailLongDowntime, config.FailCascadeStop, config.FailOpeningDelay}},
		{"continuous maps brief_stop", config.KindContinuous, dhs, []string{config.FailChainStop, config.FailLongDowntime}},
		{"continuous already listed", config.KindContinuous, synthetic, []string{config.FailLongDowntime, config.FailChainStop}},
		{"elevator", config.KindElevator, dhs, []string{config.FailBriefStop, config.FailLongDowntime}},
		{"bays", config.KindBays, dhs, []string{config.FailBriefStop, config.FailLongDowntime}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := rideFailures(c.kind, c.scenario); !slices.Equal(got, c.want) {
				t.Errorf("rideFailures = %v, want %v", got, c.want)
			}
		})
	}
}
