package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/stretchr/testify/mock"
)

func TestEnsureBotUserCachesBotID(t *testing.T) {
	t.Parallel()

	api := &plugintest.API{}
	defer api.AssertExpectations(t)

	api.On("EnsureBotUser", mock.MatchedBy(func(bot *model.Bot) bool {
		return bot.Username == botUsername &&
			bot.DisplayName == botDisplayName &&
			bot.Description == botDescription
	})).Return("bot-user-id", nil).Once()

	p := &Plugin{}
	p.API = api

	if err := p.ensureBotUser(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got := p.botUserID(); got != "bot-user-id" {
		t.Fatalf("expected cached bot ID %q, got %q", "bot-user-id", got)
	}
}

func TestEnsureBotUserClearsBotIDOnError(t *testing.T) {
	t.Parallel()

	api := &plugintest.API{}
	defer api.AssertExpectations(t)

	api.On("EnsureBotUser", mock.AnythingOfType("*model.Bot")).Return("", errors.New("bot create failed")).Once()

	p := &Plugin{}
	p.API = api
	p.setBotUserID("old-bot-id")

	err := p.ensureBotUser()
	if err == nil {
		t.Fatal("expected error")
	}
	if got := p.botUserID(); got != "" {
		t.Fatalf("expected bot ID to be cleared, got %q", got)
	}
}

func TestSendDenialMessageUsesCachedBotUser(t *testing.T) {
	t.Parallel()

	api := &plugintest.API{}
	defer api.AssertExpectations(t)

	api.On("GetDirectChannel", "target-user-id", "bot-user-id").Return(&model.Channel{Id: "dm-channel-id"}, nil).Once()
	api.On("CreatePost", mock.MatchedBy(func(post *model.Post) bool {
		return post.UserId == "bot-user-id" &&
			post.ChannelId == "dm-channel-id" &&
			strings.Contains(post.Message, "classified") &&
			strings.Contains(post.Message, "Denied by policy")
	})).Return(&model.Post{Id: "post-id"}, nil).Once()

	p := &Plugin{}
	p.API = api
	p.setBotUserID("bot-user-id")

	p.sendDenialMessage("target-user-id", "classified", "Denied by policy")
}

func TestSendDenialMessageSkipsWithoutBotUser(t *testing.T) {
	t.Parallel()

	api := &plugintest.API{}
	defer api.AssertExpectations(t)
	api.On("LogError", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return().Once()

	p := &Plugin{}
	p.API = api

	p.sendDenialMessage("target-user-id", "classified", "Denied by policy")
}
