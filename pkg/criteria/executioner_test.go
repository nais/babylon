package criteria

import (
	"github.com/nais/babylon/pkg/config"
	promconfig "github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/timeinterval"
	"gopkg.in/yaml.v2"
	"testing"
	"time"
)

func TestConfig_InActivePeriod(t *testing.T) {
	t.Parallel()

	cases := []struct {
		Name     string
		In       string
		Times    []time.Time
		Expected []bool
	}{
		{
			Name:     "Always valid",
			In:       "",
			Times:    []time.Time{time.Now()},
			Expected: []bool{true},
		},
		{
			Name: "Valid for default working hours",
			In: `
    - name: working-hours
      time_intervals:
        - weekdays: ["monday:friday"]
          times:
          - start_time: 10:00
            end_time: 17:00
`,
			Times: []time.Time{
				time.Date(2021, time.July, 1, 10, 0, 0, 0, time.UTC),
				time.Date(2021, time.August, 2, 16, 0, 0, 0, time.UTC),
			},
			Expected: []bool{true, true},
		},
		{
			Name: "Invalid when outside working hours",
			In: `
    - name: working-hours
      time_intervals:
        - weekdays: ["monday:friday"]
          times:
          - start_time: 10:00
            end_time: 17:00
`,
			Times: []time.Time{
				time.Date(2021, time.July, 3, 10, 0, 0, 0, time.UTC),
				time.Date(2021, time.August, 1, 16, 0, 0, 0, time.UTC),
			},
			Expected: []bool{false, false},
		},
		{
			Name: "Only valid on mondays",
			In: `
    - name: working-hours
      time_intervals:
        - weekdays: ["monday"]
          times:
          - start_time: 10:00
            end_time: 17:00
`,
			Times: []time.Time{
				time.Date(2021, time.July, 3, 10, 0, 0, 0, time.UTC),
				time.Date(2021, time.August, 1, 16, 0, 0, 0, time.UTC),
				time.Date(2021, time.August, 2, 16, 0, 0, 0, time.UTC),
			},
			Expected: []bool{false, false, true},
		},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()

			cfg := config.DefaultConfig()
			if tt.In != "" {
				var intervals []promconfig.MuteTimeInterval
				_ = yaml.Unmarshal([]byte(tt.In), &intervals)
				cfg.ActiveTimeIntervals = map[string][]timeinterval.TimeInterval{}
				for _, mti := range intervals {
					cfg.ActiveTimeIntervals[mti.Name] = mti.TimeIntervals
				}
			}

			executioner := NewExecutioner(&cfg, nil, nil, nil)

			for i, timings := range tt.Times {
				if executioner.inActivePeriod(timings) != tt.Expected[i] {
					t.Fatalf("Expected %v to be in active period: %+v", timings, cfg.ActiveTimeIntervals)
				}
			}
		})
	}
}
