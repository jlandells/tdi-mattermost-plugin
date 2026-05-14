package main

import (
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
)

// Regression test for the bug where the hook returned the incoming
// *model.Config even when no mutation was intended. The Mattermost server
// treats a non-nil return as "replace the config with this one" — and because
// the value has been round-tripped through this plugin's (possibly older)
// model.Config definitions, any fields the plugin doesn't know about (e.g.
// newer AutoTranslationSettings.Agents) are silently dropped, corrupting the
// saved config and breaking every subsequent System Console save.
func TestConfigurationWillBeSavedReturnsNilWhenDisabled(t *testing.T) {
	t.Parallel()

	p := &Plugin{}
	p.setConfiguration(&configuration{EnableConfigValidationPolicy: false})

	cfg, err := p.ConfigurationWillBeSaved(&model.Config{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg != nil {
		t.Fatal("expected nil *model.Config so the server keeps its original config; returning the round-tripped parameter strips fields the plugin's struct doesn't know about")
	}
}
