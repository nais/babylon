package config

import (
	"testing"
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
