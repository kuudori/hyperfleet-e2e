package helper

import (
	"strings"
	"testing"
)

func TestPatchEntityRequiredAdapters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		vals     apiValues
		kind     string
		adapters []string
		wantErr  string
	}{
		{
			name: "patches matching entity",
			vals: apiValues{
				Config: struct {
					Entities []apiEntity    `yaml:"entities"`
					Rest     map[string]any `yaml:",inline"`
				}{
					Entities: []apiEntity{
						{Kind: "Cluster", RequiredAdapters: []string{"old-adapter"}},
					},
				},
			},
			kind:     "Cluster",
			adapters: []string{"new-a", "new-b"},
		},
		{
			name: "entity kind not found",
			vals: apiValues{
				Config: struct {
					Entities []apiEntity    `yaml:"entities"`
					Rest     map[string]any `yaml:",inline"`
				}{
					Entities: []apiEntity{
						{Kind: "NodePool"},
					},
				},
			},
			kind:     "Cluster",
			adapters: []string{"a"},
			wantErr:  `entity with kind "Cluster" not found`,
		},
		{
			name:     "empty entities",
			vals:     apiValues{},
			kind:     "Cluster",
			adapters: []string{"a"},
			wantErr:  `entity with kind "Cluster" not found`,
		},
		{
			name: "multiple entities patches correct one",
			vals: apiValues{
				Config: struct {
					Entities []apiEntity    `yaml:"entities"`
					Rest     map[string]any `yaml:",inline"`
				}{
					Entities: []apiEntity{
						{Kind: "NodePool", RequiredAdapters: []string{"np-adapter"}},
						{Kind: "Cluster", RequiredAdapters: []string{"old"}},
					},
				},
			},
			kind:     "Cluster",
			adapters: []string{"new-a", "new-b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := patchEntityRequiredAdapters(&tt.vals, tt.kind, tt.adapters)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %q, want containing %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Find the patched entity and verify
			for _, e := range tt.vals.Config.Entities {
				if e.Kind != tt.kind {
					continue
				}
				if len(e.RequiredAdapters) != len(tt.adapters) {
					t.Fatalf("patched adapters len = %d, want %d", len(e.RequiredAdapters), len(tt.adapters))
				}
				for i, want := range tt.adapters {
					if e.RequiredAdapters[i] != want {
						t.Errorf("adapter[%d] = %v, want %v", i, e.RequiredAdapters[i], want)
					}
				}
				return
			}
			t.Fatal("patched entity not found after successful patch")
		})
	}
}
