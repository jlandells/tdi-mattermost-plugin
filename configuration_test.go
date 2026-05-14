package main

import (
	"strings"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/stretchr/testify/mock"
)

func TestValidateConfiguration(t *testing.T) {
	t.Parallel()

	valid := &configuration{
		TDIURL:              "https://tdi.example.com",
		TDINamespace:        "mattermost-policies",
		PolicyTimeout:       5,
		EnableMessagePolicy: true,
	}

	tests := []struct {
		name    string
		config  *configuration
		wantErr string
	}{
		{name: "valid", config: valid},
		{name: "default install config without policy service", config: &configuration{}},
		{name: "nil", config: nil, wantErr: "configuration is required"},
		{
			name: "missing TDI URL with policy enabled",
			config: &configuration{
				TDINamespace:        "mattermost-policies",
				PolicyTimeout:       5,
				EnableMessagePolicy: true,
			},
			wantErr: "TDIURL is required when policy checks are enabled",
		},
		{
			name: "relative TDI URL",
			config: &configuration{
				TDIURL:              "tdi.example.com",
				TDINamespace:        "mattermost-policies",
				PolicyTimeout:       5,
				EnableMessagePolicy: true,
			},
			wantErr: "TDIURL must be an absolute URL",
		},
		{
			name: "missing namespace with policy enabled",
			config: &configuration{
				TDIURL:              "https://tdi.example.com",
				PolicyTimeout:       5,
				EnableMessagePolicy: true,
			},
			wantErr: "TDINamespace is required when policy checks are enabled",
		},
		{
			name: "negative timeout",
			config: &configuration{
				TDIURL:              "https://tdi.example.com",
				TDINamespace:        "mattermost-policies",
				PolicyTimeout:       -1,
				EnableMessagePolicy: true,
			},
			wantErr: "PolicyTimeout must be zero or greater",
		},
		{
			name: "excessive timeout",
			config: &configuration{
				TDIURL:              "https://tdi.example.com",
				TDINamespace:        "mattermost-policies",
				PolicyTimeout:       61,
				EnableMessagePolicy: true,
			},
			wantErr: "PolicyTimeout must be 60 seconds or less",
		},
		{
			name: "invalid attribute mapping",
			config: &configuration{
				TDIURL:               "https://tdi.example.com",
				TDINamespace:         "mattermost-policies",
				PolicyTimeout:        5,
				EnableMessagePolicy:  true,
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
				EnableMessagePolicy:    true,
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
				EnableMessagePolicy:    true,
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

func TestResolveScopedTeams(t *testing.T) {
	t.Parallel()

	t.Run("empty list leaves nil ID set", func(t *testing.T) {
		t.Parallel()
		api := &plugintest.API{}
		defer api.AssertExpectations(t)

		p := &Plugin{}
		p.API = api
		cfg := &configuration{}
		p.resolveScopedTeams(cfg)

		if cfg.scopedTeamIDs != nil {
			t.Fatalf("expected nil scopedTeamIDs, got %v", cfg.scopedTeamIDs)
		}
	})

	t.Run("normalises whitespace, case, and duplicates", func(t *testing.T) {
		t.Parallel()
		api := &plugintest.API{}
		defer api.AssertExpectations(t)

		api.On("GetTeamByName", "engineering").Return(&model.Team{Id: "team-eng"}, nil).Once()
		api.On("GetTeamByName", "sales-emea").Return(&model.Team{Id: "team-sales"}, nil).Once()

		p := &Plugin{}
		p.API = api
		cfg := &configuration{
			ScopedTeamNames: []string{"Engineering", "  engineering ", "sales-emea", ""},
		}
		p.resolveScopedTeams(cfg)

		if got := cfg.ScopedTeamNames; len(got) != 2 || got[0] != "engineering" || got[1] != "sales-emea" {
			t.Fatalf("expected normalised [engineering sales-emea], got %v", got)
		}
		if _, ok := cfg.scopedTeamIDs["team-eng"]; !ok {
			t.Fatal("expected team-eng in resolved IDs")
		}
		if _, ok := cfg.scopedTeamIDs["team-sales"]; !ok {
			t.Fatal("expected team-sales in resolved IDs")
		}
	})

	t.Run("unknown name logs a warning and is skipped", func(t *testing.T) {
		t.Parallel()
		api := &plugintest.API{}
		defer api.AssertExpectations(t)

		api.On("GetTeamByName", "engineering").Return(&model.Team{Id: "team-eng"}, nil).Once()
		api.On("GetTeamByName", "made-up").Return((*model.Team)(nil), &model.AppError{Message: "not found"}).Once()
		api.On("LogWarn", mock.Anything, mock.Anything, mock.Anything).Return().Once()

		p := &Plugin{}
		p.API = api
		cfg := &configuration{ScopedTeamNames: []string{"engineering", "made-up"}}
		p.resolveScopedTeams(cfg)

		if _, ok := cfg.scopedTeamIDs["team-eng"]; !ok {
			t.Fatal("expected team-eng to resolve despite the unknown sibling")
		}
		if len(cfg.scopedTeamIDs) != 1 {
			t.Fatalf("expected exactly one resolved team, got %d", len(cfg.scopedTeamIDs))
		}
	})
}

func TestConfigurationCloneDeepCopiesScope(t *testing.T) {
	t.Parallel()

	original := &configuration{
		ScopedTeamNames: []string{"engineering"},
		scopedTeamIDs:   map[string]struct{}{"team-eng": {}},
	}
	clone := original.Clone()

	clone.ScopedTeamNames[0] = "sales"
	clone.scopedTeamIDs["team-sales"] = struct{}{}

	if original.ScopedTeamNames[0] != "engineering" {
		t.Fatalf("Clone did not deep-copy ScopedTeamNames; original mutated to %v", original.ScopedTeamNames)
	}
	if _, leaked := original.scopedTeamIDs["team-sales"]; leaked {
		t.Fatal("Clone did not deep-copy scopedTeamIDs; original mutated")
	}
}
