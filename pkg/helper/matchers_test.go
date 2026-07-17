package helper

import (
	"testing"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/util"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
)

func TestHaveAuditIdentity(t *testing.T) {
	expected := "system:serviceaccount:hyperfleet:hyperfleet-e2e-sa"
	other := "someone-else@example.com"

	tests := []struct {
		name      string
		actual    any
		wantMatch bool
		wantErr   bool
	}{
		{
			name:      "Resource with matching identity",
			actual:    &client.Resource{CreatedBy: util.ToPtr(expected)},
			wantMatch: true,
		},
		{
			name:      "Resource with mismatched identity",
			actual:    &client.Resource{CreatedBy: util.ToPtr(other)},
			wantMatch: false,
		},
		{
			name:    "nil *Resource",
			actual:  (*client.Resource)(nil),
			wantErr: true,
		},
		{
			name:      "Resource with nil CreatedBy",
			actual:    &client.Resource{CreatedBy: nil},
			wantMatch: false,
		},
		{
			name:    "unsupported type",
			actual:  "not-a-cluster",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := HaveAuditIdentity(expected)
			matched, err := matcher.Match(tt.actual)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got match=%v", matched)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if matched != tt.wantMatch {
				t.Errorf("got match=%v, want %v", matched, tt.wantMatch)
			}

			if !matched {
				msg := matcher.FailureMessage(tt.actual)
				if msg == "" {
					t.Error("FailureMessage returned empty string")
				}
			}
		})
	}
}
