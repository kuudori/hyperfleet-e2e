package helper

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

// hashSuffixPattern matches the deterministic hash appended on truncation:
// dash followed by exactly 8 lowercase hex chars at end of string.
var hashSuffixPattern = regexp.MustCompile(`-[0-9a-f]{8}$`)

// TestGenerateAdapterReleaseName covers the main behavioral properties:
// - short names pass through unchanged
// - names exactly at the limit pass through unchanged
// - over-limit names get truncated AND get a hash suffix
// - output never exceeds the max length
func TestGenerateAdapterReleaseName(t *testing.T) {
	tests := []struct {
		name         string
		resourceType string
		adapterName  string
		wantExact    string // when set, assert exact equality
		wantHashed   bool   // expect hash suffix at the end
	}{
		{
			name:         "short name, no truncation",
			resourceType: "clusters",
			adapterName:  "validation",
			wantExact:    "adapter-clusters-validation",
			wantHashed:   false,
		},
		{
			name:         "typical adapter name",
			resourceType: "nodepools",
			adapterName:  "maestro",
			wantExact:    "adapter-nodepools-maestro",
			wantHashed:   false,
		},
		{
			name:         "exactly at limit, no truncation",
			resourceType: "clusters",
			adapterName:  strings.Repeat("x", maxReleaseNameLength-len("adapter-clusters-")),
			wantExact:    "adapter-clusters-" + strings.Repeat("x", maxReleaseNameLength-len("adapter-clusters-")),
			wantHashed:   false,
		},
		{
			name:         "over limit, truncated with hash",
			resourceType: "clusters",
			adapterName:  "my-very-long-adapter-name-that-exceeds-the-limit",
			wantHashed:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateAdapterReleaseName(tt.resourceType, tt.adapterName)

			if len(got) > maxReleaseNameLength {
				t.Errorf("length %d exceeds max %d: %q", len(got), maxReleaseNameLength, got)
			}

			wantPrefix := fmt.Sprintf("adapter-%s-", tt.resourceType)
			if !strings.HasPrefix(got, wantPrefix) {
				t.Errorf("missing prefix %q in %q", wantPrefix, got)
			}

			hasHash := hashSuffixPattern.MatchString(got)
			if hasHash != tt.wantHashed {
				t.Errorf("hash suffix presence: got %v, want %v (name: %q)", hasHash, tt.wantHashed, got)
			}

			if tt.wantExact != "" && got != tt.wantExact {
				t.Errorf("got %q, want %q", got, tt.wantExact)
			}
		})
	}
}

// TestGenerateAdapterReleaseName_Deterministic asserts that release names are
// fully deterministic: identical inputs must always produce identical outputs.
// This is a hard requirement — uninstall flows locate releases by exact name,
// and any randomness here would cause cleanup to leak orphan Helm releases.
func TestGenerateAdapterReleaseName_Deterministic(t *testing.T) {
	cases := []struct {
		resourceType string
		adapterName  string
	}{
		{"clusters", "foo"},
		{"nodepools", "maestro"},
		{"clusters", "my-very-long-adapter-name-that-exceeds-the-limit"},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s-%s", tc.resourceType, tc.adapterName), func(t *testing.T) {
			a := GenerateAdapterReleaseName(tc.resourceType, tc.adapterName)
			b := GenerateAdapterReleaseName(tc.resourceType, tc.adapterName)
			if a != b {
				t.Errorf("non-deterministic output: %q != %q", a, b)
			}
		})
	}
}

// TestGenerateAdapterReleaseName_LongNameCollision asserts that two distinct
// long names sharing a long common prefix produce distinct release names.
// The hash suffix is what guarantees uniqueness once the base is truncated.
func TestGenerateAdapterReleaseName_LongNameCollision(t *testing.T) {
	a := GenerateAdapterReleaseName("clusters", "adapter-alpha-very-long-name-here")
	b := GenerateAdapterReleaseName("clusters", "adapter-bravo-very-long-name-here")

	if a == b {
		t.Errorf("different inputs produced same release name: %q", a)
	}
}
