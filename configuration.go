package main

import (
	"encoding/json"
	"net/url"
	"reflect"
	"strings"

	"github.com/pkg/errors"
)

const (
	defaultPolicyTimeout          = 5
	defaultMaxFileInspectionBytes = 100 * 1024 * 1024
	maxFileInspectionBytesLimit   = 1024 * 1024 * 1024
)

// configuration captures the plugin's external configuration as exposed in the Mattermost server
// configuration, as well as values computed from the configuration. Any public fields will be
// deserialized from the Mattermost server configuration in OnConfigurationChange.
//
// As plugins are inherently concurrent (hooks being called asynchronously), and the plugin
// configuration can change at any time, access to the configuration must be synchronized. The
// strategy used in this plugin is to guard a pointer to the configuration, and clone the entire
// struct whenever it changes. You may replace this with whatever strategy you choose.
//
// If you add non-reference types to your configuration struct, be sure to rewrite Clone as a deep
// copy appropriate for your types.
type configuration struct {
	TDIURL                       string
	TDINamespace                 string
	TDIAPIKey                    string
	EnableMessagePolicy          bool
	EnableChannelJoinPolicy      bool
	EnableMessageEditPolicy      bool
	EnableMessageDeletePolicy    bool
	EnableFileUploadPolicy       bool
	EnableLoginPolicy            bool
	EnableChannelCreationPolicy  bool
	EnableReactionPolicy         bool
	EnableUserCreatedPolicy      bool
	EnableTeamJoinPolicy         bool
	EnableUserLeftTeamPolicy     bool
	EnableUserLeftChannelPolicy  bool
	EnableMessagePostedPolicy    bool // audit: MessageHasBeenPosted
	EnableMessageUpdatedPolicy   bool // audit: MessageHasBeenUpdated
	EnableUserLoggedInPolicy     bool // audit: UserHasLoggedIn
	EnableMessagesConsumedPolicy bool // filter: MessagesWillBeConsumed
	EnableUserDeactivatedPolicy  bool // audit: UserHasBeenDeactivated
	EnablePushNotificationPolicy bool // block/modify: NotificationWillBePushed
	EnableConfigValidationPolicy bool // validate: ConfigurationWillBeSaved
	EnableSAMLLoginPolicy        bool // audit: OnSAMLLogin (server 10.7+)
	PolicyTimeout                int
	MaxFileInspectionBytes       int64
	EnableDebugLogging           bool
	ExemptSystemAdmins           bool
	UserAttributeMapping         string
	// MattermostAPIToken is used for REST calls to Mattermost (e.g. access control policy search/assign).
	// Required for the classify-channel feature. Use a Personal Access Token or System Admin token.
	MattermostAPIToken string
	// ScopedTeamNames is the list of team URL slugs (Name) this plugin's policy
	// enforcement is restricted to. Empty / nil means apply to all teams.
	ScopedTeamNames []string

	// scopedTeamIDs is the resolved set of team IDs corresponding to
	// ScopedTeamNames, populated in OnConfigurationChange. Empty when
	// ScopedTeamNames is empty (the "all teams" case).
	scopedTeamIDs map[string]struct{}
}

// Clone produces a deep copy safe to mutate without affecting the original.
func (c *configuration) Clone() *configuration {
	clone := *c
	if c.ScopedTeamNames != nil {
		clone.ScopedTeamNames = append([]string(nil), c.ScopedTeamNames...)
	}
	if c.scopedTeamIDs != nil {
		clone.scopedTeamIDs = make(map[string]struct{}, len(c.scopedTeamIDs))
		for id := range c.scopedTeamIDs {
			clone.scopedTeamIDs[id] = struct{}{}
		}
	}
	return &clone
}

// getConfiguration retrieves the active configuration under lock, making it safe to use
// concurrently. The active configuration may change underneath the client of this method, but
// the struct returned by this API call is considered immutable.
func (p *Plugin) getConfiguration() *configuration {
	p.configurationLock.RLock()
	defer p.configurationLock.RUnlock()

	if p.configuration == nil {
		return &configuration{}
	}

	return p.configuration
}

// setConfiguration replaces the active configuration under lock.
//
// Do not call setConfiguration while holding the configurationLock, as sync.Mutex is not
// reentrant. In particular, avoid using the plugin API entirely, as this may in turn trigger a
// hook back into the plugin. If that hook attempts to acquire this lock, a deadlock may occur.
//
// This method panics if setConfiguration is called with the existing configuration. This almost
// certainly means that the configuration was modified without being cloned and may result in
// an unsafe access.
func (p *Plugin) setConfiguration(configuration *configuration) {
	p.configurationLock.Lock()
	defer p.configurationLock.Unlock()

	if configuration != nil && p.configuration == configuration {
		// Ignore assignment if the configuration struct is empty. Go will optimize the
		// allocation for same to point at the same memory address, breaking the check
		// above.
		if reflect.ValueOf(*configuration).NumField() == 0 {
			return
		}

		panic("setConfiguration called with the existing configuration")
	}

	p.configuration = configuration
}

// OnConfigurationChange is invoked when configuration changes may have been made.
func (p *Plugin) OnConfigurationChange() error {
	var configuration = new(configuration)

	// Load the public configuration fields from the Mattermost server configuration.
	if err := p.API.LoadPluginConfiguration(configuration); err != nil {
		return errors.Wrap(err, "failed to load plugin configuration")
	}

	if err := p.validateConfiguration(configuration); err != nil {
		return errors.Wrap(err, "invalid plugin configuration")
	}

	p.resolveScopedTeams(configuration)

	p.setConfiguration(configuration)

	return nil
}

// resolveScopedTeams normalises ScopedTeamNames and resolves each name to a
// team ID via the Mattermost API. Unknown teams are logged and skipped so a
// typo doesn't break the entire plugin. Mutates configuration in place.
func (p *Plugin) resolveScopedTeams(configuration *configuration) {
	configuration.scopedTeamIDs = nil

	seen := make(map[string]struct{})
	normalised := make([]string, 0, len(configuration.ScopedTeamNames))
	for _, raw := range configuration.ScopedTeamNames {
		name := strings.ToLower(strings.TrimSpace(raw))
		if name == "" {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		normalised = append(normalised, name)
	}
	configuration.ScopedTeamNames = normalised

	if len(normalised) == 0 {
		return
	}

	ids := make(map[string]struct{}, len(normalised))
	for _, name := range normalised {
		team, appErr := p.API.GetTeamByName(name)
		if appErr != nil || team == nil {
			p.logWarn("Scoped team name not found; ignoring", "team_name", name)
			continue
		}
		ids[team.Id] = struct{}{}
	}
	configuration.scopedTeamIDs = ids
}

// validateConfiguration validates the configuration and returns an error if it is invalid.
func (p *Plugin) validateConfiguration(configuration *configuration) error {
	if configuration == nil {
		return errors.New("configuration is required")
	}

	if configuration.PolicyTimeout < 0 {
		return errors.New("PolicyTimeout must be zero or greater")
	}

	if configuration.PolicyTimeout > 60 {
		return errors.New("PolicyTimeout must be 60 seconds or less")
	}

	if configuration.policyServiceEnabled() {
		if err := validatePolicyServiceConfiguration(configuration); err != nil {
			return err
		}
	}

	if configuration.MaxFileInspectionBytes < 0 {
		return errors.New("MaxFileInspectionBytes must be zero or greater")
	}

	if configuration.MaxFileInspectionBytes > maxFileInspectionBytesLimit {
		return errors.Errorf("MaxFileInspectionBytes must be %d bytes or less", maxFileInspectionBytesLimit)
	}

	if configuration.UserAttributeMapping != "" {
		var attrMapping map[string]string
		if err := json.Unmarshal([]byte(configuration.UserAttributeMapping), &attrMapping); err != nil {
			return errors.Wrap(err, "UserAttributeMapping must be valid JSON object mapping strings to strings")
		}
	}

	return nil
}

func validatePolicyServiceConfiguration(configuration *configuration) error {
	if configuration == nil {
		return errors.New("configuration is required")
	}

	if strings.TrimSpace(configuration.TDIURL) == "" {
		return errors.New("TDIURL is required when policy checks are enabled")
	}

	tdiURL, err := url.ParseRequestURI(configuration.TDIURL)
	if err != nil || tdiURL.Scheme == "" || tdiURL.Host == "" {
		return errors.New("TDIURL must be an absolute URL")
	}

	if strings.TrimSpace(configuration.TDINamespace) == "" {
		return errors.New("TDINamespace is required when policy checks are enabled")
	}

	return nil
}

func (c *configuration) maxFileInspectionBytes() int64 {
	if c == nil || c.MaxFileInspectionBytes == 0 {
		return defaultMaxFileInspectionBytes
	}
	return c.MaxFileInspectionBytes
}

func (c *configuration) policyTimeout() int {
	if c == nil || c.PolicyTimeout <= 0 {
		return defaultPolicyTimeout
	}
	return c.PolicyTimeout
}

func (c *configuration) policyServiceEnabled() bool {
	if c == nil {
		return false
	}

	return c.EnableMessagePolicy ||
		c.EnableChannelJoinPolicy ||
		c.EnableMessageEditPolicy ||
		c.EnableMessageDeletePolicy ||
		c.EnableFileUploadPolicy ||
		c.EnableLoginPolicy ||
		c.EnableChannelCreationPolicy ||
		c.EnableReactionPolicy ||
		c.EnableUserCreatedPolicy ||
		c.EnableTeamJoinPolicy ||
		c.EnableUserLeftTeamPolicy ||
		c.EnableUserLeftChannelPolicy ||
		c.EnableMessagePostedPolicy ||
		c.EnableMessageUpdatedPolicy ||
		c.EnableUserLoggedInPolicy ||
		c.EnableMessagesConsumedPolicy ||
		c.EnableUserDeactivatedPolicy ||
		c.EnablePushNotificationPolicy ||
		c.EnableConfigValidationPolicy ||
		c.EnableSAMLLoginPolicy
}
