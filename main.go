package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	saml2 "github.com/mattermost/gosaml2"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

// Plugin implements the interface expected by the Mattermost server to communicate between the server and plugin processes.
type Plugin struct {
	plugin.MattermostPlugin
	configuration     *configuration
	configurationLock sync.RWMutex
	httpClient        *http.Client
}

// PolicyRequest represents a request sent to TDI for policy decision
type PolicyRequest struct {
	UserID         string                 `json:"user_id"`
	Username       string                 `json:"username"`
	Email          string                 `json:"email"`
	ChannelID      string                 `json:"channel_id"`
	ChannelName    string                 `json:"channel_name"`
	TeamID         string                 `json:"team_id"`
	TeamName       string                 `json:"team_name"`
	Message        string                 `json:"message,omitempty"`
	UserAttributes map[string]interface{} `json:"user_attributes"`
	ChannelHeader  string                 `json:"channel_header,omitempty"`
	Action         string                 `json:"action"` // "message" or "channel_join"
}

// PolicyResponse represents the response from TDI
type PolicyResponse struct {
	Status string                 `json:"status"`
	Action string                 `json:"action"` // "continue" or "reject"
	Result map[string]interface{} `json:"result"`
}

// OnActivate is invoked when the plugin is activated
func (p *Plugin) OnActivate() error {
	config := p.getConfiguration()

	// Initialize HTTP client with timeout
	p.httpClient = &http.Client{
		Timeout: time.Duration(config.PolicyTimeout) * time.Second,
	}

	p.API.LogInfo("Mattermost Policy Plugin activated",
		"tdi_url", config.TDIURL,
		"namespace", config.TDINamespace,
		"message_policy_enabled", config.EnableMessagePolicy,
		"channel_join_policy_enabled", config.EnableChannelJoinPolicy,
	)

	return nil
}

// MessageWillBePosted is invoked when a message is posted by a user before it is committed
// to the database. This allows the plugin to reject messages based on policy decisions.
func (p *Plugin) MessageWillBePosted(c *plugin.Context, post *model.Post) (*model.Post, string) {
	config := p.getConfiguration()

	// Skip if message policy is disabled
	if !config.EnableMessagePolicy {
		return post, ""
	}

	// Get user info
	user, err := p.API.GetUser(post.UserId)
	if err != nil {
		p.API.LogError("Failed to get user", "error", err.Error())
		return nil, "Failed to verify user permissions"
	}

	// Exempt system admins if configured
	if config.ExemptSystemAdmins && user.IsSystemAdmin() {
		if config.EnableDebugLogging {
			p.API.LogDebug("Exempting system admin from policy check", "user_id", user.Id)
		}
		return post, ""
	}

	// Get channel info
	channel, err := p.API.GetChannel(post.ChannelId)
	if err != nil {
		p.API.LogError("Failed to get channel", "error", err.Error())
		return nil, "Failed to verify channel permissions"
	}

	// Get team info
	var teamName string
	if channel.TeamId != "" {
		team, err := p.API.GetTeam(channel.TeamId)
		if err == nil {
			teamName = team.Name
		}
	}

	// Build policy request
	policyReq := PolicyRequest{
		UserID:         user.Id,
		Username:       user.Username,
		Email:          user.Email,
		ChannelID:      channel.Id,
		ChannelName:    channel.Name,
		TeamID:         channel.TeamId,
		TeamName:       teamName,
		Message:        post.Message,
		UserAttributes: p.extractUserAttributes(user),
		ChannelHeader:  channel.Header,
		Action:         "message",
	}

	// Check policy
	allowed, reason := p.checkPolicy(policyReq, "message")
	if !allowed {
		p.API.LogInfo("Message denied by policy",
			"user", user.Username,
			"channel", channel.Name,
			"reason", reason,
		)
		return nil, reason
	}

	if config.EnableDebugLogging {
		p.API.LogDebug("Message allowed by policy",
			"user", user.Username,
			"channel", channel.Name,
		)
	}

	return post, ""
}

// UserHasJoinedChannel is invoked after a user has joined a channel
// Note: Mattermost doesn't have a "before join" hook, so we need to handle this differently
// This demonstrates the pattern - for true prevention, you'd need to use channel membership hooks
func (p *Plugin) UserHasJoinedChannel(c *plugin.Context, channelMember *model.ChannelMember, actor *model.User) {
	config := p.getConfiguration()

	// Skip if channel join policy is disabled
	if !config.EnableChannelJoinPolicy {
		return
	}

	// Get user info
	user, err := p.API.GetUser(channelMember.UserId)
	if err != nil {
		p.API.LogError("Failed to get user", "error", err.Error())
		return
	}

	// Exempt system admins if configured
	if config.ExemptSystemAdmins && user.IsSystemAdmin() {
		if config.EnableDebugLogging {
			p.API.LogDebug("Exempting system admin from channel join policy", "user_id", user.Id)
		}
		return
	}

	// Get channel info
	channel, err := p.API.GetChannel(channelMember.ChannelId)
	if err != nil {
		p.API.LogError("Failed to get channel", "error", err.Error())
		return
	}

	// Get team info
	var teamName string
	if channel.TeamId != "" {
		team, err := p.API.GetTeam(channel.TeamId)
		if err == nil {
			teamName = team.Name
		}
	}

	// Build policy request
	policyReq := PolicyRequest{
		UserID:         user.Id,
		Username:       user.Username,
		Email:          user.Email,
		ChannelID:      channel.Id,
		ChannelName:    channel.Name,
		TeamID:         channel.TeamId,
		TeamName:       teamName,
		UserAttributes: p.extractUserAttributes(user),
		ChannelHeader:  channel.Header,
		Action:         "channel_join",
	}

	// Check policy
	allowed, reason := p.checkPolicy(policyReq, "channel_join")
	if !allowed {
		p.API.LogInfo("Channel join denied by policy - removing user",
			"user", user.Username,
			"channel", channel.Name,
			"reason", reason,
		)

		// Remove user from channel
		err := p.API.DeleteChannelMember(channel.Id, user.Id)
		if err != nil {
			p.API.LogError("Failed to remove user from channel", "error", err.Error())
		}

		// Send DM to user explaining denial
		p.sendDenialMessage(user.Id, channel.Name, reason)
		return
	}

	if config.EnableDebugLogging {
		p.API.LogDebug("Channel join allowed by policy",
			"user", user.Username,
			"channel", channel.Name,
		)
	}
}

// checkPolicy calls TDI to check if an action is allowed
func (p *Plugin) checkPolicy(req PolicyRequest, policyType string) (bool, string) {
	config := p.getConfiguration()

	// Validate configuration
	if config.TDIURL == "" || config.TDINamespace == "" {
		p.API.LogError("TDI not configured - denying by default (fail-secure)")
		return false, "Policy service not configured"
	}

	// Build TDI URL
	var endpoint string
	if policyType == "message" {
		endpoint = fmt.Sprintf("%s/ns/%s/policy/v1/message/check",
			strings.TrimSuffix(config.TDIURL, "/"),
			config.TDINamespace)
	} else {
		endpoint = fmt.Sprintf("%s/ns/%s/policy/v1/channel/join",
			strings.TrimSuffix(config.TDIURL, "/"),
			config.TDINamespace)
	}

	// Marshal request
	jsonData, err := json.Marshal(req)
	if err != nil {
		p.API.LogError("Failed to marshal policy request", "error", err.Error())
		return false, "Internal error processing policy request"
	}

	if config.EnableDebugLogging {
		p.API.LogDebug("Sending policy request", "endpoint", endpoint, "payload", string(jsonData))
	}

	// Create HTTP request
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.PolicyTimeout)*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		p.API.LogError("Failed to create HTTP request", "error", err.Error())
		return false, "Internal error contacting policy service"
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if config.TDIAPIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+config.TDIAPIKey)
	}

	// Send request
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		p.API.LogError("Failed to contact TDI", "error", err.Error(), "endpoint", endpoint)
		return false, "Policy service unavailable"
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		p.API.LogError("Failed to read policy response", "error", err.Error())
		return false, "Error reading policy response"
	}

	if config.EnableDebugLogging {
		p.API.LogDebug("Policy response received", "status", resp.StatusCode, "body", string(body))
	}

	// Check HTTP status
	if resp.StatusCode != http.StatusOK {
		p.API.LogError("Policy service returned error", "status", resp.StatusCode, "body", string(body))
		return false, "Policy service error"
	}

	// Parse response
	var policyResp PolicyResponse
	if err := json.Unmarshal(body, &policyResp); err != nil {
		p.API.LogError("Failed to parse policy response", "error", err.Error())
		return false, "Invalid policy response"
	}

	// Check response
	if policyResp.Action == "continue" {
		return true, ""
	}

	// Extract denial reason
	reason := "Access denied by policy"
	if reasonStr, ok := policyResp.Result["reason"].(string); ok && reasonStr != "" {
		reason = reasonStr
	}

	return false, reason
}

// extractUserAttributes extracts user attributes from various sources for TDI policy evaluation.
// - Basic fields (username, email, roles, etc.) are always included.
// - Built-in User fields (first_name, last_name, nickname, position) are included so SAML-mapped
//   attributes flow through (SAML populates these, not user.Props).
// - Custom profile attributes (Mattermost Enterprise v10.10+, e.g. SecurityClearance, Nationality)
//   are fetched via SearchPropertyValues / SearchPropertyFields and included by field name.
//   These sync from SAML/LDAP when linked in System Console > System properties.
// - UserAttributeMapping maps policy keys to Mattermost/LDAP fields. Supports:
//   - Built-in User fields: first_name, last_name, nickname, position
//   - user.Props (custom data written by other plugins/integrations)
//   - LDAP attributes (when AuthService=ldap): uses GetLDAPUserAttributes
func (p *Plugin) extractUserAttributes(user *model.User) map[string]interface{} {
	config := p.getConfiguration()
	attributes := make(map[string]interface{})

	// Basic attributes (always included)
	attributes["username"] = user.Username
	attributes["email"] = user.Email
	attributes["roles"] = user.Roles
	attributes["is_system_admin"] = user.IsSystemAdmin()

	// Built-in User fields populated by SAML/IdP (first name, last name, nickname, position)
	attributes["first_name"] = user.FirstName
	attributes["last_name"] = user.LastName
	attributes["nickname"] = user.Nickname
	attributes["position"] = user.Position

	// Custom profile attributes (Enterprise v10.10+): SecurityClearance, Nationality, etc.
	// Same data as GET /api/v4/users/{id}/custom_profile_attributes
	p.mergeCustomProfileAttributes(user.Id, attributes)

	// Custom attribute mapping from configuration
	var attrMapping map[string]string
	if config.UserAttributeMapping != "" {
		if err := json.Unmarshal([]byte(config.UserAttributeMapping), &attrMapping); err == nil {
			builtInFields := map[string]string{
				"first_name": user.FirstName, "last_name": user.LastName,
				"nickname": user.Nickname, "position": user.Position,
				"username": user.Username, "email": user.Email,
			}
			for key, mmField := range attrMapping {
				if mmField == "" {
					continue
				}
				// Built-in User fields
				if val, ok := builtInFields[mmField]; ok && val != "" {
					attributes[key] = val
					continue
				}
				// user.Props (custom data from other plugins/integrations)
				if val, ok := user.Props[mmField]; ok {
					attributes[key] = val
					continue
				}
			}
			// LDAP users: pull attributes directly from LDAP (Enterprise, LDAP configured)
			if user.AuthService == "ldap" {
				builtInNames := map[string]bool{"first_name": true, "last_name": true, "nickname": true, "position": true, "username": true, "email": true}
				ldapAttrs := make([]string, 0, len(attrMapping))
				for _, mmField := range attrMapping {
					if mmField != "" && !builtInNames[mmField] {
						ldapAttrs = append(ldapAttrs, mmField)
					}
				}
				if len(ldapAttrs) > 0 {
					if ldapMap, err := p.API.GetLDAPUserAttributes(user.Id, ldapAttrs); err == nil && ldapMap != nil {
						for key, mmField := range attrMapping {
							if v, ok := ldapMap[mmField]; ok && v != "" {
								attributes[key] = v
							}
						}
					}
				}
			}
		}
	}

	return attributes
}

// mergeCustomProfileAttributes fetches custom profile attributes (e.g. SecurityClearance, Nationality)
// from Mattermost Enterprise System properties and merges them into attributes by field name.
// Requires Mattermost v10.10+ and Enterprise. Same data as GET /api/v4/users/{id}/custom_profile_attributes.
func (p *Plugin) mergeCustomProfileAttributes(userID string, attributes map[string]interface{}) {
	group, err := p.API.GetPropertyGroup(model.CustomProfileAttributesPropertyGroupName)
	if err != nil || group == nil || group.ID == "" {
		return
	}
	opts := model.PropertyValueSearchOpts{
		GroupID:    group.ID,
		TargetType: model.PropertyValueTargetTypeUser,
		TargetIDs:  []string{userID},
		PerPage:    50,
	}
	values, err := p.API.SearchPropertyValues(group.ID, opts)
	if err != nil || len(values) == 0 {
		return
	}
	fieldOpts := model.PropertyFieldSearchOpts{GroupID: group.ID, PerPage: 50}
	fields, err := p.API.SearchPropertyFields(group.ID, fieldOpts)
	if err != nil || len(fields) == 0 {
		return
	}
	fieldIDToName := make(map[string]string)
	for _, f := range fields {
		if f != nil && f.ID != "" && f.Name != "" {
			fieldIDToName[f.ID] = f.Name
		}
	}
	for _, pv := range values {
		if pv == nil || pv.FieldID == "" {
			continue
		}
		name := fieldIDToName[pv.FieldID]
		if name == "" {
			name = pv.FieldID // fallback to ID if name unknown
		}
		var val string
		if len(pv.Value) > 0 {
			_ = json.Unmarshal(pv.Value, &val)
		}
		if val != "" {
			attributes[name] = val
		}
	}
}

// sendDenialMessage sends a DM to a user explaining why their action was denied
func (p *Plugin) sendDenialMessage(userID, channelName, reason string) {
	// Get or create DM channel with user
	dmChannel, err := p.API.GetDirectChannel(userID, p.botUserID())
	if err != nil {
		p.API.LogError("Failed to get DM channel", "error", err.Error())
		return
	}

	message := fmt.Sprintf("You were removed from channel **%s** because: %s", channelName, reason)

	post := &model.Post{
		UserId:    p.botUserID(),
		ChannelId: dmChannel.Id,
		Message:   message,
	}

	if _, err := p.API.CreatePost(post); err != nil {
		p.API.LogError("Failed to send denial message", "error", err.Error())
	}
}

// botUserID returns the bot user ID (you'd need to create a bot user for the plugin)
func (p *Plugin) botUserID() string {
	// In a real implementation, you'd create a bot user on activation
	// and store its ID. For simplicity, using empty string here.
	// The plugin should use p.API.CreateBot() to create a proper bot
	return ""
}

// ============================================================================
// HIGH-VALUE HOOKS - 5 NEW POLICY CONTROLS
// ============================================================================

// MessageWillBeUpdated is invoked when a message is updated by a user before
// the update is committed to the database
func (p *Plugin) MessageWillBeUpdated(c *plugin.Context, newPost, oldPost *model.Post) (*model.Post, string) {
	config := p.getConfiguration()

	// Skip if message edit policy is disabled
	if !config.EnableMessageEditPolicy {
		return newPost, ""
	}

	// Get user info
	user, err := p.API.GetUser(newPost.UserId)
	if err != nil {
		p.API.LogError("Failed to get user", "error", err.Error())
		return oldPost, "Failed to verify user permissions"
	}

	// Exempt system admins if configured
	if config.ExemptSystemAdmins && user.IsSystemAdmin() {
		if config.EnableDebugLogging {
			p.API.LogDebug("Exempting system admin from edit policy check", "user_id", user.Id)
		}
		return newPost, ""
	}

	// Get channel info
	channel, err := p.API.GetChannel(newPost.ChannelId)
	if err != nil {
		p.API.LogError("Failed to get channel", "error", err.Error())
		return oldPost, "Failed to verify channel permissions"
	}

	// Calculate time since original post
	timeSincePost := time.Since(time.Unix(oldPost.CreateAt/1000, 0))

	// Build policy request
	policyReq := map[string]interface{}{
		"user_id":          user.Id,
		"username":         user.Username,
		"email":            user.Email,
		"channel_id":       channel.Id,
		"channel_name":     channel.Name,
		"channel_header":   channel.Header,
		"old_message":      oldPost.Message,
		"new_message":      newPost.Message,
		"post_age_seconds": int(timeSincePost.Seconds()),
		"user_attributes":  p.extractUserAttributes(user),
		"action":           "message_edit",
	}

	// Check policy
	allowed, reason := p.checkGenericPolicy(policyReq, "message/edit")
	if !allowed {
		p.API.LogInfo("Message edit denied by policy",
			"user", user.Username,
			"channel", channel.Name,
			"reason", reason,
		)
		return oldPost, reason // Return original post
	}

	if config.EnableDebugLogging {
		p.API.LogDebug("Message edit allowed by policy",
			"user", user.Username,
			"channel", channel.Name,
		)
	}

	return newPost, ""
}

// MessageHasBeenDeleted is invoked after a message has been deleted from the database.
// Mattermost does not provide a MessageWillBeDeleted hook, so this is audit-only; deletions cannot be blocked.
func (p *Plugin) MessageHasBeenDeleted(c *plugin.Context, post *model.Post) {
	config := p.getConfiguration()
	if !config.EnableMessageDeletePolicy {
		return
	}

	user, err := p.API.GetUser(post.UserId)
	if err != nil {
		p.API.LogError("Failed to get user for message delete audit", "error", err.Error())
		return
	}

	if config.ExemptSystemAdmins && user.IsSystemAdmin() {
		if config.EnableDebugLogging {
			p.API.LogDebug("Exempting system admin from message delete audit", "user_id", user.Id)
		}
		return
	}

	channel, err := p.API.GetChannel(post.ChannelId)
	if err != nil {
		p.API.LogError("Failed to get channel for message delete audit", "error", err.Error())
		return
	}

	timeSincePost := time.Since(time.Unix(post.CreateAt/1000, 0))
	policyReq := map[string]interface{}{
		"user_id":          user.Id,
		"username":         user.Username,
		"email":            user.Email,
		"channel_id":       channel.Id,
		"channel_name":     channel.Name,
		"channel_header":   channel.Header,
		"message":          post.Message,
		"post_age_seconds": int(timeSincePost.Seconds()),
		"user_attributes":  p.extractUserAttributes(user),
		"action":           "message_delete",
	}

	_, _ = p.checkGenericPolicy(policyReq, "message/delete")
	if config.EnableDebugLogging {
		p.API.LogDebug("Message deletion reported to TDI for audit", "user", user.Username, "channel", channel.Name)
	}
}

// FileWillBeUploaded is invoked before a file is uploaded
func (p *Plugin) FileWillBeUploaded(c *plugin.Context, info *model.FileInfo, file io.Reader, output io.Writer) (*model.FileInfo, string) {
	config := p.getConfiguration()

	// Skip if file upload policy is disabled
	if !config.EnableFileUploadPolicy {
		io.Copy(output, file)
		return info, ""
	}

	// Get user info
	user, err := p.API.GetUser(info.CreatorId)
	if err != nil {
		p.API.LogError("Failed to get user", "error", err.Error())
		return nil, "Failed to verify user permissions"
	}

	// Exempt system admins if configured
	if config.ExemptSystemAdmins && user.IsSystemAdmin() {
		if config.EnableDebugLogging {
			p.API.LogDebug("Exempting system admin from file upload policy check", "user_id", user.Id)
		}
		io.Copy(output, file)
		return info, ""
	}

	// Get channel info (if available)
	var channel *model.Channel
	var channelName, channelHeader string
	if info.ChannelId != "" {
		var channelErr *model.AppError
		channel, channelErr = p.API.GetChannel(info.ChannelId)
		if channelErr == nil {
			channelName = channel.Name
			channelHeader = channel.Header
		}
	}

	// Read file data for policy check (and to compute hash)
	fileData, readErr := io.ReadAll(file)
	if readErr != nil {
		p.API.LogError("Failed to read file", "error", readErr.Error())
		return nil, "Failed to process file"
	}

	// Compute file hash
	hash := sha256.Sum256(fileData)
	fileHash := fmt.Sprintf("%x", hash)

	// Build policy request
	policyReq := map[string]interface{}{
		"user_id":         user.Id,
		"username":        user.Username,
		"email":           user.Email,
		"channel_id":      info.ChannelId,
		"channel_name":    channelName,
		"channel_header":  channelHeader,
		"filename":        info.Name,
		"file_size":       info.Size,
		"mime_type":       info.MimeType,
		"file_extension":  filepath.Ext(info.Name),
		"file_hash":       fileHash,
		"user_attributes": p.extractUserAttributes(user),
		"action":          "file_upload",
	}

	// Check policy
	allowed, reason := p.checkGenericPolicy(policyReq, "file/upload")
	if !allowed {
		p.API.LogInfo("File upload denied by policy",
			"user", user.Username,
			"filename", info.Name,
			"reason", reason,
		)
		return nil, reason
	}

	// Write file to output
	_, writeErr := io.Copy(output, bytes.NewReader(fileData))
	if writeErr != nil {
		p.API.LogError("Failed to write file", "error", writeErr.Error())
		return nil, "Failed to process file"
	}

	if config.EnableDebugLogging {
		p.API.LogDebug("File upload allowed by policy",
			"user", user.Username,
			"filename", info.Name,
		)
	}

	return info, ""
}

// UserWillLogIn is invoked before a user logs in
func (p *Plugin) UserWillLogIn(c *plugin.Context, user *model.User) string {
	config := p.getConfiguration()

	// Skip if login policy is disabled
	if !config.EnableLoginPolicy {
		return ""
	}

	// Exempt system admins if configured
	if config.ExemptSystemAdmins && user.IsSystemAdmin() {
		if config.EnableDebugLogging {
			p.API.LogDebug("Exempting system admin from login policy check", "user_id", user.Id)
		}
		return ""
	}

	// Build policy request
	policyReq := map[string]interface{}{
		"user_id":         user.Id,
		"username":        user.Username,
		"email":           user.Email,
		"user_attributes": p.extractUserAttributes(user),
		"login_time":      time.Now().Format(time.RFC3339),
		"action":          "user_login",
	}

	// Check policy
	allowed, reason := p.checkGenericPolicy(policyReq, "user/login")
	if !allowed {
		p.API.LogInfo("User login denied by policy",
			"user", user.Username,
			"reason", reason,
		)
		return reason
	}

	if config.EnableDebugLogging {
		p.API.LogDebug("User login allowed by policy",
			"user", user.Username,
		)
	}

	return ""
}

// ChannelHasBeenCreated is invoked after a channel has been created
func (p *Plugin) ChannelHasBeenCreated(c *plugin.Context, channel *model.Channel) {
	config := p.getConfiguration()

	// Skip if channel creation policy is disabled
	if !config.EnableChannelCreationPolicy {
		return
	}

	// Get creator info
	user, err := p.API.GetUser(channel.CreatorId)
	if err != nil {
		p.API.LogError("Failed to get channel creator", "error", err.Error())
		return
	}

	// Exempt system admins if configured
	if config.ExemptSystemAdmins && user.IsSystemAdmin() {
		if config.EnableDebugLogging {
			p.API.LogDebug("Exempting system admin from channel creation policy", "user_id", user.Id)
		}
		return
	}

	// Build policy request
	policyReq := map[string]interface{}{
		"user_id":         user.Id,
		"username":        user.Username,
		"email":           user.Email,
		"channel_id":      channel.Id,
		"channel_name":    channel.Name,
		"channel_type":    channel.Type,
		"channel_header":  channel.Header,
		"user_attributes": p.extractUserAttributes(user),
		"action":          "channel_create",
	}

	// Check policy
	allowed, reason := p.checkGenericPolicy(policyReq, "channel/create")
	if !allowed {
		p.API.LogInfo("Channel creation policy violation - taking action",
			"user", user.Username,
			"channel", channel.Name,
			"reason", reason,
		)

		// Auto-classify or modify channel based on policy
		// For example, add clearance requirement to header
		if strings.Contains(strings.ToLower(channel.Name), "secret") {
			channel.Header = "CLEARANCE_REQUIRED=SECRET"
			p.API.UpdateChannel(channel)
		}

		// Notify creator
		p.sendChannelCreationNotice(user.Id, channel.Name, reason)
		return
	}

	// Auto-classification logic
	classification := p.classifyChannelByName(channel.Name)
	if classification != "" {
		channel.Header = fmt.Sprintf("CLEARANCE_REQUIRED=%s", classification)
		p.API.UpdateChannel(channel)

		if config.EnableDebugLogging {
			p.API.LogDebug("Auto-classified channel",
				"channel", channel.Name,
				"classification", classification,
			)
		}
	}
}

// classifyChannelByName determines classification based on channel name
func (p *Plugin) classifyChannelByName(name string) string {
	nameLower := strings.ToLower(name)

	if strings.Contains(nameLower, "ts-") || strings.Contains(nameLower, "topsecret") {
		return "TOP SECRET"
	}
	if strings.Contains(nameLower, "secret") || strings.Contains(nameLower, "classified") {
		return "SECRET"
	}
	if strings.Contains(nameLower, "confidential") {
		return "CONFIDENTIAL"
	}

	return ""
}

// sendChannelCreationNotice sends a DM about channel creation policy
func (p *Plugin) sendChannelCreationNotice(userID, channelName, notice string) {
	dmChannel, err := p.API.GetDirectChannel(userID, p.botUserID())
	if err != nil {
		p.API.LogError("Failed to get DM channel", "error", err.Error())
		return
	}

	message := fmt.Sprintf("**Channel Creation Notice**\n\nChannel: **%s**\n\n%s", channelName, notice)

	post := &model.Post{
		UserId:    p.botUserID(),
		ChannelId: dmChannel.Id,
		Message:   message,
	}

	if _, err := p.API.CreatePost(post); err != nil {
		p.API.LogError("Failed to send channel creation notice", "error", err.Error())
	}
}

// ============================================================================
// ADDITIONAL HOOKS - Reactions, Team, User, Channel lifecycle
// ============================================================================

// ReactionHasBeenAdded is invoked after a reaction has been added. We check policy
// and remove the reaction if TDI rejects (e.g. restrict reactions in classified channels).
func (p *Plugin) ReactionHasBeenAdded(c *plugin.Context, reaction *model.Reaction) {
	config := p.getConfiguration()
	if !config.EnableReactionPolicy {
		return
	}
	user, err := p.API.GetUser(reaction.UserId)
	if err != nil {
		return
	}
	if config.ExemptSystemAdmins && user.IsSystemAdmin() {
		return
	}
	post, err := p.API.GetPost(reaction.PostId)
	if err != nil {
		return
	}
	channel, _ := p.API.GetChannel(post.ChannelId)
	channelName := ""
	channelHeader := ""
	if channel != nil {
		channelName = channel.Name
		channelHeader = channel.Header
	}
	policyReq := map[string]interface{}{
		"user_id":         user.Id,
		"username":        user.Username,
		"channel_id":      post.ChannelId,
		"channel_name":    channelName,
		"channel_header":  channelHeader,
		"emoji_name":      reaction.EmojiName,
		"user_attributes": p.extractUserAttributes(user),
		"action":          "reaction_add",
	}
	allowed, reason := p.checkGenericPolicy(policyReq, "reaction/add")
	if !allowed {
		p.API.LogInfo("Reaction denied by policy - removing", "user", user.Username, "emoji", reaction.EmojiName, "reason", reason)
		_ = p.API.RemoveReaction(reaction)
	}
}

// ReactionHasBeenRemoved is invoked after a reaction has been removed (audit only).
func (p *Plugin) ReactionHasBeenRemoved(c *plugin.Context, reaction *model.Reaction) {
	config := p.getConfiguration()
	if !config.EnableReactionPolicy {
		return
	}
	policyReq := map[string]interface{}{
		"user_id":    reaction.UserId,
		"post_id":    reaction.PostId,
		"emoji_name": reaction.EmojiName,
		"action":     "reaction_remove",
	}
	_, _ = p.checkGenericPolicy(policyReq, "reaction/remove")
}

// UserHasBeenCreated is invoked after a new user has been created (audit / onboarding).
func (p *Plugin) UserHasBeenCreated(c *plugin.Context, user *model.User) {
	config := p.getConfiguration()
	if !config.EnableUserCreatedPolicy {
		return
	}
	policyReq := map[string]interface{}{
		"user_id":         user.Id,
		"username":        user.Username,
		"email":           user.Email,
		"user_attributes": p.extractUserAttributes(user),
		"action":          "user_created",
	}
	_, _ = p.checkGenericPolicy(policyReq, "user/created")
}

// UserHasJoinedTeam is invoked after a user has joined a team. We check policy and remove if rejected.
func (p *Plugin) UserHasJoinedTeam(c *plugin.Context, teamMember *model.TeamMember, actor *model.User) {
	config := p.getConfiguration()
	if !config.EnableTeamJoinPolicy {
		return
	}
	user, err := p.API.GetUser(teamMember.UserId)
	if err != nil {
		return
	}
	if config.ExemptSystemAdmins && user.IsSystemAdmin() {
		return
	}
	team, _ := p.API.GetTeam(teamMember.TeamId)
	teamName := ""
	if team != nil {
		teamName = team.Name
	}
	policyReq := map[string]interface{}{
		"user_id":         user.Id,
		"username":        user.Username,
		"team_id":         teamMember.TeamId,
		"team_name":       teamName,
		"user_attributes": p.extractUserAttributes(user),
		"action":          "team_join",
	}
	allowed, reason := p.checkGenericPolicy(policyReq, "team/join")
	if !allowed {
		p.API.LogInfo("Team join denied by policy - removing user", "user", user.Username, "team", teamName, "reason", reason)
		_ = p.API.DeleteTeamMember(teamMember.TeamId, teamMember.UserId, "")
		if p.botUserID() != "" {
			p.sendDenialMessage(user.Id, "team "+teamName, reason)
		}
	}
}

// UserHasLeftTeam is invoked after a user has left a team (audit only).
func (p *Plugin) UserHasLeftTeam(c *plugin.Context, teamMember *model.TeamMember, actor *model.User) {
	config := p.getConfiguration()
	if !config.EnableUserLeftTeamPolicy {
		return
	}
	team, _ := p.API.GetTeam(teamMember.TeamId)
	teamName := ""
	if team != nil {
		teamName = team.Name
	}
	policyReq := map[string]interface{}{
		"user_id":   teamMember.UserId,
		"team_id":   teamMember.TeamId,
		"team_name": teamName,
		"action":    "team_leave",
	}
	_, _ = p.checkGenericPolicy(policyReq, "team/leave")
}

// UserHasLeftChannel is invoked after a user has left a channel (audit only).
func (p *Plugin) UserHasLeftChannel(c *plugin.Context, channelMember *model.ChannelMember, actor *model.User) {
	config := p.getConfiguration()
	if !config.EnableUserLeftChannelPolicy {
		return
	}
	channel, _ := p.API.GetChannel(channelMember.ChannelId)
	channelName := ""
	if channel != nil {
		channelName = channel.Name
	}
	policyReq := map[string]interface{}{
		"user_id":     channelMember.UserId,
		"channel_id":  channelMember.ChannelId,
		"channel_name": channelName,
		"action":      "channel_leave",
	}
	_, _ = p.checkGenericPolicy(policyReq, "channel/leave")
}

// ============================================================================
// OPTIONAL HOOKS - Message/User audit, MessagesWillBeConsumed, File download,
// UserDeactivated, Push notification, Config validation, SAML login
// ============================================================================

// MessageHasBeenPosted is invoked after a message has been committed to the database (audit).
func (p *Plugin) MessageHasBeenPosted(c *plugin.Context, post *model.Post) {
	config := p.getConfiguration()
	if !config.EnableMessagePostedPolicy {
		return
	}
	user, err := p.API.GetUser(post.UserId)
	if err != nil {
		return
	}
	channel, _ := p.API.GetChannel(post.ChannelId)
	channelName, channelHeader := "", ""
	if channel != nil {
		channelName, channelHeader = channel.Name, channel.Header
	}
	policyReq := map[string]interface{}{
		"user_id":         user.Id,
		"username":        user.Username,
		"channel_id":      post.ChannelId,
		"channel_name":    channelName,
		"channel_header":  channelHeader,
		"message":         post.Message,
		"user_attributes": p.extractUserAttributes(user),
		"action":          "message_posted",
	}
	_, _ = p.checkGenericPolicy(policyReq, "message/posted")
}

// MessageHasBeenUpdated is invoked after a message has been updated (audit).
func (p *Plugin) MessageHasBeenUpdated(c *plugin.Context, newPost, oldPost *model.Post) {
	config := p.getConfiguration()
	if !config.EnableMessageUpdatedPolicy {
		return
	}
	user, err := p.API.GetUser(newPost.UserId)
	if err != nil {
		return
	}
	channel, _ := p.API.GetChannel(newPost.ChannelId)
	channelName, channelHeader := "", ""
	if channel != nil {
		channelName, channelHeader = channel.Name, channel.Header
	}
	policyReq := map[string]interface{}{
		"user_id":         user.Id,
		"username":        user.Username,
		"channel_id":      newPost.ChannelId,
		"channel_name":    channelName,
		"channel_header":  channelHeader,
		"old_message":     oldPost.Message,
		"new_message":     newPost.Message,
		"user_attributes": p.extractUserAttributes(user),
		"action":          "message_updated",
	}
	_, _ = p.checkGenericPolicy(policyReq, "message/updated")
}

// UserHasLoggedIn is invoked after a user has logged in (audit).
func (p *Plugin) UserHasLoggedIn(c *plugin.Context, user *model.User) {
	config := p.getConfiguration()
	if !config.EnableUserLoggedInPolicy {
		return
	}
	policyReq := map[string]interface{}{
		"user_id":         user.Id,
		"username":        user.Username,
		"email":           user.Email,
		"user_attributes": p.extractUserAttributes(user),
		"login_time":      time.Now().Format(time.RFC3339),
		"action":          "user_logged_in",
	}
	_, _ = p.checkGenericPolicy(policyReq, "user/logged_in")
}

// MessagesWillBeConsumed is invoked when messages are requested by a client before they are returned.
// Returns filtered posts. When policy is enabled, reports to TDI and returns posts unchanged (audit).
// For DLP/clearance filtering, TDI would need to return post IDs to exclude; requires server 9.3+.
func (p *Plugin) MessagesWillBeConsumed(posts []*model.Post) []*model.Post {
	config := p.getConfiguration()
	if !config.EnableMessagesConsumedPolicy || len(posts) == 0 {
		return posts
	}
	postIDs := make([]string, 0, len(posts))
	for _, post := range posts {
		if post != nil && post.Id != "" {
			postIDs = append(postIDs, post.Id)
		}
	}
	policyReq := map[string]interface{}{
		"post_ids":   postIDs,
		"post_count": len(posts),
		"action":     "messages_consumed",
	}
	_, _ = p.checkGenericPolicy(policyReq, "messages/consumed")
	return posts
}

// UserHasBeenDeactivated is invoked when a user is deactivated (audit). Requires server 9.1+.
func (p *Plugin) UserHasBeenDeactivated(c *plugin.Context, user *model.User) {
	config := p.getConfiguration()
	if !config.EnableUserDeactivatedPolicy {
		return
	}
	policyReq := map[string]interface{}{
		"user_id":         user.Id,
		"username":        user.Username,
		"email":           user.Email,
		"user_attributes": p.extractUserAttributes(user),
		"action":          "user_deactivated",
	}
	_, _ = p.checkGenericPolicy(policyReq, "user/deactivated")
}

// NotificationWillBePushed is invoked before a push notification is sent. Return non-empty string to reject.
// Requires server 9.0+.
func (p *Plugin) NotificationWillBePushed(pushNotification *model.PushNotification, userID string) (*model.PushNotification, string) {
	config := p.getConfiguration()
	if !config.EnablePushNotificationPolicy {
		return nil, ""
	}
	policyReq := map[string]interface{}{
		"user_id":     userID,
		"post_id":     pushNotification.PostId,
		"channel_id":  pushNotification.ChannelId,
		"server_id":   pushNotification.ServerId,
		"message":     pushNotification.Message,
		"action":      "push_notification",
	}
	allowed, reason := p.checkGenericPolicy(policyReq, "notification/push")
	if !allowed {
		return nil, reason
	}
	return nil, ""
}

// ConfigurationWillBeSaved is invoked before saving the server configuration.
// Return error to reject the save. Requires server 8.0+.
func (p *Plugin) ConfigurationWillBeSaved(newCfg *model.Config) (*model.Config, error) {
	config := p.getConfiguration()
	if !config.EnableConfigValidationPolicy {
		return newCfg, nil
	}
	policyReq := map[string]interface{}{
		"action": "config_will_be_saved",
	}
	allowed, reason := p.checkGenericPolicy(policyReq, "config/validate")
	if !allowed {
		return nil, fmt.Errorf("config validation rejected by policy: %s", reason)
	}
	return newCfg, nil
}

// OnSAMLLogin is invoked after a successful SAML login (audit). Requires server 10.7+.
func (p *Plugin) OnSAMLLogin(c *plugin.Context, user *model.User, assertion *saml2.AssertionInfo) error {
	config := p.getConfiguration()
	if !config.EnableSAMLLoginPolicy {
		return nil
	}
	policyReq := map[string]interface{}{
		"user_id":         user.Id,
		"username":        user.Username,
		"email":           user.Email,
		"user_attributes": p.extractUserAttributes(user),
		"action":          "saml_login",
	}
	_, _ = p.checkGenericPolicy(policyReq, "saml/login")
	return nil
}

// checkGenericPolicy checks a generic policy with any request structure
func (p *Plugin) checkGenericPolicy(req map[string]interface{}, policyPath string) (bool, string) {
	config := p.getConfiguration()

	// Validate configuration
	if config.TDIURL == "" || config.TDINamespace == "" {
		p.API.LogError("TDI not configured - denying by default (fail-secure)")
		return false, "Policy service not configured"
	}

	// Build TDI URL
	endpoint := fmt.Sprintf("%s/ns/%s/policy/v1/%s",
		strings.TrimSuffix(config.TDIURL, "/"),
		config.TDINamespace,
		policyPath)

	// Marshal request
	jsonData, err := json.Marshal(req)
	if err != nil {
		p.API.LogError("Failed to marshal policy request", "error", err.Error())
		return false, "Internal error processing policy request"
	}

	if config.EnableDebugLogging {
		p.API.LogDebug("Sending policy request", "endpoint", endpoint, "payload", string(jsonData))
	}

	// Create HTTP request
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.PolicyTimeout)*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		p.API.LogError("Failed to create HTTP request", "error", err.Error())
		return false, "Internal error contacting policy service"
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if config.TDIAPIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+config.TDIAPIKey)
	}

	// Send request
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		p.API.LogError("Failed to contact TDI", "error", err.Error(), "endpoint", endpoint)
		return false, "Policy service unavailable"
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		p.API.LogError("Failed to read policy response", "error", err.Error())
		return false, "Error reading policy response"
	}

	if config.EnableDebugLogging {
		p.API.LogDebug("Policy response received", "status", resp.StatusCode, "body", string(body))
	}

	// Check HTTP status
	if resp.StatusCode != http.StatusOK {
		p.API.LogError("Policy service returned error", "status", resp.StatusCode, "body", string(body))
		return false, "Policy service error"
	}

	// Parse response
	var policyResp PolicyResponse
	if err := json.Unmarshal(body, &policyResp); err != nil {
		p.API.LogError("Failed to parse policy response", "error", err.Error())
		return false, "Invalid policy response"
	}

	// Check response
	if policyResp.Action == "continue" {
		return true, ""
	}

	// Extract denial reason
	reason := "Access denied by policy"
	if reasonStr, ok := policyResp.Result["reason"].(string); ok && reasonStr != "" {
		reason = reasonStr
	}

	return false, reason
}

func main() {
	plugin.ClientMain(&Plugin{})
}
