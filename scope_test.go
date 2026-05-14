package main

import (
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/stretchr/testify/mock"
)

func TestTeamInScope(t *testing.T) {
	t.Parallel()

	t.Run("empty scope is open", func(t *testing.T) {
		t.Parallel()
		c := &configuration{}
		if !c.teamInScope("any-team-id") {
			t.Fatal("empty scope should admit any team")
		}
		if !c.teamInScope("") {
			t.Fatal("empty scope should admit empty team id (no scoping configured)")
		}
	})

	t.Run("non-empty scope admits only listed team IDs", func(t *testing.T) {
		t.Parallel()
		c := &configuration{
			scopedTeamIDs: map[string]struct{}{
				"team-alpha": {},
				"team-beta":  {},
			},
		}
		if !c.teamInScope("team-alpha") {
			t.Fatal("expected team-alpha in scope")
		}
		if c.teamInScope("team-gamma") {
			t.Fatal("expected team-gamma out of scope")
		}
		if c.teamInScope("") {
			t.Fatal("empty team id should never be in scope when scope is set")
		}
	})

	t.Run("nil configuration is open", func(t *testing.T) {
		t.Parallel()
		var c *configuration
		if !c.teamInScope("anything") {
			t.Fatal("nil config should be treated as no scoping")
		}
	})
}

func TestUserInScope(t *testing.T) {
	t.Parallel()

	t.Run("empty scope is open without API call", func(t *testing.T) {
		t.Parallel()
		api := &plugintest.API{}
		defer api.AssertExpectations(t)

		p := &Plugin{}
		p.API = api
		p.setConfiguration(&configuration{})

		if !p.userInScope("user-id") {
			t.Fatal("empty scope should admit any user")
		}
	})

	t.Run("user in a scoped team is admitted", func(t *testing.T) {
		t.Parallel()
		api := &plugintest.API{}
		defer api.AssertExpectations(t)

		api.On("GetTeamsForUser", "user-id").Return([]*model.Team{
			{Id: "team-gamma"},
			{Id: "team-alpha"},
		}, nil).Once()

		p := &Plugin{}
		p.API = api
		p.setConfiguration(&configuration{
			scopedTeamIDs: map[string]struct{}{"team-alpha": {}, "team-beta": {}},
		})

		if !p.userInScope("user-id") {
			t.Fatal("expected user to be in scope via team-alpha membership")
		}
	})

	t.Run("user in no scoped team is excluded", func(t *testing.T) {
		t.Parallel()
		api := &plugintest.API{}
		defer api.AssertExpectations(t)

		api.On("GetTeamsForUser", "user-id").Return([]*model.Team{
			{Id: "team-gamma"},
		}, nil).Once()

		p := &Plugin{}
		p.API = api
		p.setConfiguration(&configuration{
			scopedTeamIDs: map[string]struct{}{"team-alpha": {}},
		})

		if p.userInScope("user-id") {
			t.Fatal("expected user out of scope")
		}
	})

	t.Run("API error fails open", func(t *testing.T) {
		t.Parallel()
		api := &plugintest.API{}
		defer api.AssertExpectations(t)

		api.On("GetTeamsForUser", "user-id").Return(([]*model.Team)(nil), &model.AppError{Message: "boom"}).Once()
		api.On("LogWarn", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return().Once()

		p := &Plugin{}
		p.API = api
		p.setConfiguration(&configuration{
			scopedTeamIDs: map[string]struct{}{"team-alpha": {}},
		})

		if !p.userInScope("user-id") {
			t.Fatal("API error should fail open (return true)")
		}
	})

	t.Run("empty user id with non-empty scope is excluded", func(t *testing.T) {
		t.Parallel()
		api := &plugintest.API{}
		defer api.AssertExpectations(t)

		p := &Plugin{}
		p.API = api
		p.setConfiguration(&configuration{
			scopedTeamIDs: map[string]struct{}{"team-alpha": {}},
		})

		if p.userInScope("") {
			t.Fatal("empty user id should never be in scope when scope is set")
		}
	})
}

func TestChannelInScope(t *testing.T) {
	t.Parallel()

	t.Run("empty scope is open without API call", func(t *testing.T) {
		t.Parallel()
		api := &plugintest.API{}
		defer api.AssertExpectations(t)

		p := &Plugin{}
		p.API = api
		p.setConfiguration(&configuration{})

		if !p.channelInScope("channel-id", "user-id") {
			t.Fatal("empty scope should admit any channel")
		}
	})

	t.Run("channel in a scoped team is admitted", func(t *testing.T) {
		t.Parallel()
		api := &plugintest.API{}
		defer api.AssertExpectations(t)

		api.On("GetChannel", "channel-id").Return(&model.Channel{Id: "channel-id", TeamId: "team-alpha"}, nil).Once()

		p := &Plugin{}
		p.API = api
		p.setConfiguration(&configuration{
			scopedTeamIDs: map[string]struct{}{"team-alpha": {}},
		})

		if !p.channelInScope("channel-id", "user-id") {
			t.Fatal("expected channel in team-alpha to be in scope")
		}
	})

	t.Run("channel out of scope is excluded", func(t *testing.T) {
		t.Parallel()
		api := &plugintest.API{}
		defer api.AssertExpectations(t)

		api.On("GetChannel", "channel-id").Return(&model.Channel{Id: "channel-id", TeamId: "team-gamma"}, nil).Once()

		p := &Plugin{}
		p.API = api
		p.setConfiguration(&configuration{
			scopedTeamIDs: map[string]struct{}{"team-alpha": {}},
		})

		if p.channelInScope("channel-id", "user-id") {
			t.Fatal("expected channel in team-gamma to be out of scope")
		}
	})

	t.Run("DM falls back to actor team membership", func(t *testing.T) {
		t.Parallel()
		api := &plugintest.API{}
		defer api.AssertExpectations(t)

		// DM has empty TeamId.
		api.On("GetChannel", "dm-channel").Return(&model.Channel{Id: "dm-channel", TeamId: ""}, nil).Once()
		api.On("GetTeamsForUser", "actor-id").Return([]*model.Team{
			{Id: "team-alpha"},
		}, nil).Once()

		p := &Plugin{}
		p.API = api
		p.setConfiguration(&configuration{
			scopedTeamIDs: map[string]struct{}{"team-alpha": {}},
		})

		if !p.channelInScope("dm-channel", "actor-id") {
			t.Fatal("expected DM to be in scope via actor's team-alpha membership")
		}
	})

	t.Run("DM with actor outside scope is excluded", func(t *testing.T) {
		t.Parallel()
		api := &plugintest.API{}
		defer api.AssertExpectations(t)

		api.On("GetChannel", "dm-channel").Return(&model.Channel{Id: "dm-channel", TeamId: ""}, nil).Once()
		api.On("GetTeamsForUser", "actor-id").Return([]*model.Team{
			{Id: "team-gamma"},
		}, nil).Once()

		p := &Plugin{}
		p.API = api
		p.setConfiguration(&configuration{
			scopedTeamIDs: map[string]struct{}{"team-alpha": {}},
		})

		if p.channelInScope("dm-channel", "actor-id") {
			t.Fatal("expected DM to be out of scope when actor not in any scoped team")
		}
	})

	t.Run("channel→team lookup is cached", func(t *testing.T) {
		t.Parallel()
		api := &plugintest.API{}
		defer api.AssertExpectations(t)

		// Expect exactly ONE GetChannel call across two channelInScope invocations.
		api.On("GetChannel", "channel-id").Return(&model.Channel{Id: "channel-id", TeamId: "team-alpha"}, nil).Once()

		p := &Plugin{}
		p.API = api
		p.setConfiguration(&configuration{
			scopedTeamIDs: map[string]struct{}{"team-alpha": {}},
		})

		for i := 0; i < 5; i++ {
			if !p.channelInScope("channel-id", "user-id") {
				t.Fatalf("iteration %d: expected channel in scope", i)
			}
		}
	})

	t.Run("GetChannel error fails open", func(t *testing.T) {
		t.Parallel()
		api := &plugintest.API{}
		defer api.AssertExpectations(t)

		api.On("GetChannel", "channel-id").Return((*model.Channel)(nil), &model.AppError{Message: "boom"}).Once()
		api.On("LogWarn", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return().Once()

		p := &Plugin{}
		p.API = api
		p.setConfiguration(&configuration{
			scopedTeamIDs: map[string]struct{}{"team-alpha": {}},
		})

		if !p.channelInScope("channel-id", "user-id") {
			t.Fatal("API error should fail open (return true)")
		}
	})
}
