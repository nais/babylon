package config

import (
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/timeinterval"
	"gopkg.in/yaml.v2"
	"testing"
	"time"
)

func TestConfig_IsNamespaceAllowed(t *testing.T) {
	t.Parallel()

	namespaces := []struct {
		Name                 string
		Namespace            string
		AllowedNamespaces    []string
		UseAllowedNamespaces bool
		Expected             bool
	}{
		{
			Name:                 "By default everything is allowed",
			Namespace:            "testdefault",
			AllowedNamespaces:    []string{},
			UseAllowedNamespaces: false,
			Expected:             true,
		},
		{
			Name:                 "Works on single namespace",
			Namespace:            "test",
			AllowedNamespaces:    []string{"test"},
			UseAllowedNamespaces: true,
			Expected:             true,
		},
		{
			Name:                 "Works on multiple allowed namespaces",
			Namespace:            "guri",
			AllowedNamespaces:    []string{"guri", "tor", "marianne"},
			UseAllowedNamespaces: true,
			Expected:             true,
		},
		{
			Name:                 "Works when name is contained in allowed namespace",
			Namespace:            "odd",
			AllowedNamespaces:    []string{"oddrane"},
			UseAllowedNamespaces: true,
			Expected:             true,
		},
		{
			Name:                 "Not working namespace",
			Namespace:            "notworking",
			AllowedNamespaces:    []string{"allowed"},
			UseAllowedNamespaces: true,
			Expected:             false,
		},
		{
			Name:                 "Empty allowed namespaces",
			Namespace:            "test",
			AllowedNamespaces:    []string{},
			UseAllowedNamespaces: true,
			Expected:             false,
		},
		{
			Name:                 "Sanity check",
			Namespace:            "kuttl-test-able-molly",
			AllowedNamespaces:    []string{"babylon-test", "kuttl-test"},
			UseAllowedNamespaces: true,
			Expected:             true,
		},
	}

	for _, tt := range namespaces {
		tt := tt
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()

			cfg := DefaultConfig()

			cfg.UseAllowedNamespaces = tt.UseAllowedNamespaces
			cfg.AllowedNamespaces = tt.AllowedNamespaces
			actual := cfg.IsNamespaceAllowed(tt.Namespace)

			if actual != tt.Expected {
				t.Fatalf("Expected namespace %s to be %t was %t", tt.Namespace, tt.Expected, actual)
			}
		})
	}

}

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

			cfg := DefaultConfig()
			if tt.In != "" {
				var intervals []config.MuteTimeInterval
				_ = yaml.Unmarshal([]byte(tt.In), &intervals)
				cfg.ActiveTimeIntervals = map[string][]timeinterval.TimeInterval{}
				for _, mti := range intervals {
					cfg.ActiveTimeIntervals[mti.Name] = mti.TimeIntervals
				}
			}

			for i, timings := range tt.Times {
				if cfg.InActivePeriod(timings) != tt.Expected[i] {
					t.Fatalf("Expected %v to be in active period: %+v", timings, cfg.ActiveTimeIntervals)
				}
			}
		})
	}
}
