package main

import (
	"strings"
	"testing"
)

func TestValidateConfiguration(t *testing.T) {
	t.Parallel()

	valid := &configuration{
		TDIURL:        "https://tdi.example.com",
		TDINamespace:  "mattermost-policies",
		PolicyTimeout: 5,
	}

	tests := []struct {
		name    string
		config  *configuration
		wantErr string
	}{
		{name: "valid", config: valid},
		{name: "nil", config: nil, wantErr: "configuration is required"},
		{
			name: "missing TDI URL",
			config: &configuration{
				TDINamespace:  "mattermost-policies",
				PolicyTimeout: 5,
			},
			wantErr: "TDIURL is required",
		},
		{
			name: "relative TDI URL",
			config: &configuration{
				TDIURL:        "tdi.example.com",
				TDINamespace:  "mattermost-policies",
				PolicyTimeout: 5,
			},
			wantErr: "TDIURL must be an absolute URL",
		},
		{
			name: "missing namespace",
			config: &configuration{
				TDIURL:        "https://tdi.example.com",
				PolicyTimeout: 5,
			},
			wantErr: "TDINamespace is required",
		},
		{
			name: "zero timeout",
			config: &configuration{
				TDIURL:        "https://tdi.example.com",
				TDINamespace:  "mattermost-policies",
				PolicyTimeout: 0,
			},
			wantErr: "PolicyTimeout must be greater than 0",
		},
		{
			name: "excessive timeout",
			config: &configuration{
				TDIURL:        "https://tdi.example.com",
				TDINamespace:  "mattermost-policies",
				PolicyTimeout: 61,
			},
			wantErr: "PolicyTimeout must be 60 seconds or less",
		},
		{
			name: "invalid attribute mapping",
			config: &configuration{
				TDIURL:               "https://tdi.example.com",
				TDINamespace:         "mattermost-policies",
				PolicyTimeout:        5,
				UserAttributeMapping: "{bad-json",
			},
			wantErr: "UserAttributeMapping must be valid JSON object",
		},
		{
			name: "negative file inspection size",
			config: &configuration{
				TDIURL:                 "https://tdi.example.com",
				TDINamespace:           "mattermost-policies",
				PolicyTimeout:          5,
				MaxFileInspectionBytes: -1,
			},
			wantErr: "MaxFileInspectionBytes must be zero or greater",
		},
		{
			name: "excessive file inspection size",
			config: &configuration{
				TDIURL:                 "https://tdi.example.com",
				TDINamespace:           "mattermost-policies",
				PolicyTimeout:          5,
				MaxFileInspectionBytes: maxFileInspectionBytesLimit + 1,
			},
			wantErr: "MaxFileInspectionBytes must be 1073741824 bytes or less",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := (&Plugin{}).validateConfiguration(tt.config)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("expected error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}
