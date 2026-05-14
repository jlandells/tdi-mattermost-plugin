package main

// Team scoping helpers. When the System Console "Restrict Plugin to Specific
// Teams" field is empty, every helper here returns true and the plugin behaves
// as if no scoping is configured. When it is non-empty, hooks consult these
// helpers before running their policy logic so events outside the scoped
// teams are silently ignored.

// teamInScope reports whether the given team ID falls within the configured
// scope. Empty scope ⇒ true (no filtering).
func (c *configuration) teamInScope(teamID string) bool {
	if c == nil || len(c.scopedTeamIDs) == 0 {
		return true
	}
	if teamID == "" {
		return false
	}
	_, ok := c.scopedTeamIDs[teamID]
	return ok
}

// userInScope reports whether the user is a member of at least one team in
// the configured scope. Empty scope ⇒ true. On API errors we fail open (return
// true) and log — failing closed would silently disable policy enforcement on
// every user-global event whenever the Teams API is transiently unavailable,
// which is the worse failure mode for a policy plugin.
func (p *Plugin) userInScope(userID string) bool {
	config := p.getConfiguration()
	if len(config.scopedTeamIDs) == 0 {
		return true
	}
	if userID == "" {
		return false
	}
	teams, appErr := p.API.GetTeamsForUser(userID)
	if appErr != nil {
		p.logWarn("Failed to load teams for user during scope check; failing open", "user_id", userID, "error", appErr.Error())
		return true
	}
	for _, team := range teams {
		if _, ok := config.scopedTeamIDs[team.Id]; ok {
			return true
		}
	}
	return false
}

// channelInScope resolves a channel's team and checks scope. For DM and Group
// DM channels (empty TeamId), it falls back to actorUserID's team membership.
// Empty scope ⇒ true. The channel → team ID lookup is cached on the plugin —
// channels cannot migrate teams in Mattermost so the cache never needs
// invalidation.
func (p *Plugin) channelInScope(channelID, actorUserID string) bool {
	config := p.getConfiguration()
	if len(config.scopedTeamIDs) == 0 {
		return true
	}
	if channelID == "" {
		return false
	}

	teamID, ok := p.lookupChannelTeam(channelID)
	if !ok {
		// API error already logged; fail open to match userInScope.
		return true
	}
	if teamID == "" {
		// DM / Group DM — defer to actor's team membership.
		return p.userInScope(actorUserID)
	}
	return config.teamInScope(teamID)
}

// lookupChannelTeam returns the team ID for a channel, caching the result.
// The second return value is false only if the lookup failed.
func (p *Plugin) lookupChannelTeam(channelID string) (string, bool) {
	if cached, ok := p.channelTeamCache.Load(channelID); ok {
		return cached.(string), true
	}
	channel, appErr := p.API.GetChannel(channelID)
	if appErr != nil || channel == nil {
		if appErr != nil {
			p.logWarn("Failed to resolve channel during scope check", "channel_id", channelID, "error", appErr.Error())
		}
		return "", false
	}
	p.channelTeamCache.Store(channelID, channel.TeamId)
	return channel.TeamId, true
}
