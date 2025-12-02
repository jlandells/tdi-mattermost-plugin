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

// PolicyRequest represents a request sent to Direktiv for policy decision
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

// PolicyResponse represents the response from Direktiv
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
		"direktiv_url", config.DirektivURL,
		"namespace", config.DirektivNamespace,
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

// checkPolicy calls Direktiv to check if an action is allowed
func (p *Plugin) checkPolicy(req PolicyRequest, policyType string) (bool, string) {
	config := p.getConfiguration()

	// Validate configuration
	if config.DirektivURL == "" || config.DirektivNamespace == "" {
		p.API.LogError("Direktiv not configured - denying by default (fail-secure)")
		return false, "Policy service not configured"
	}

	// Build Direktiv URL
	var endpoint string
	if policyType == "message" {
		endpoint = fmt.Sprintf("%s/ns/%s/policy/v1/message/check",
			strings.TrimSuffix(config.DirektivURL, "/"),
			config.DirektivNamespace)
	} else {
		endpoint = fmt.Sprintf("%s/ns/%s/policy/v1/channel/join",
			strings.TrimSuffix(config.DirektivURL, "/"),
			config.DirektivNamespace)
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
	if config.DirektivAPIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+config.DirektivAPIKey)
	}

	// Send request
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		p.API.LogError("Failed to contact Direktiv", "error", err.Error(), "endpoint", endpoint)
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

// extractUserAttributes extracts user attributes from various sources
func (p *Plugin) extractUserAttributes(user *model.User) map[string]interface{} {
	config := p.getConfiguration()
	attributes := make(map[string]interface{})

	// Basic attributes
	attributes["username"] = user.Username
	attributes["email"] = user.Email
	attributes["roles"] = user.Roles
	attributes["is_system_admin"] = user.IsSystemAdmin()

	// Custom attribute mapping from configuration
	var attrMapping map[string]string
	if config.UserAttributeMapping != "" {
		if err := json.Unmarshal([]byte(config.UserAttributeMapping), &attrMapping); err == nil {
			// Extract custom attributes based on mapping
			// This is a simplified version - in production, you'd read from user.Props
			for key, mmField := range attrMapping {
				// Example: read from user.Props[mmField]
				if val, ok := user.Props[mmField]; ok {
					attributes[key] = val
				}
			}
		}
	}

	// You can extend this to pull from:
	// - SAML attributes
	// - OAuth claims
	// - Custom user properties
	// - External directory services

	return attributes
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

// MessageWillBeDeleted is invoked when a message is deleted by a user before
// the deletion is committed to the database
func (p *Plugin) MessageWillBeDeleted(c *plugin.Context, post *model.Post) (*model.Post, string) {
	config := p.getConfiguration()

	// Skip if message delete policy is disabled
	if !config.EnableMessageDeletePolicy {
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
			p.API.LogDebug("Exempting system admin from delete policy check", "user_id", user.Id)
		}
		return post, ""
	}

	// Get channel info
	channel, err := p.API.GetChannel(post.ChannelId)
	if err != nil {
		p.API.LogError("Failed to get channel", "error", err.Error())
		return nil, "Failed to verify channel permissions"
	}

	// Calculate time since original post
	timeSincePost := time.Since(time.Unix(post.CreateAt/1000, 0))

	// Build policy request
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

	// Check policy
	allowed, reason := p.checkGenericPolicy(policyReq, "message/delete")
	if !allowed {
		p.API.LogInfo("Message deletion denied by policy",
			"user", user.Username,
			"channel", channel.Name,
			"reason", reason,
		)
		return nil, reason // Deny deletion
	}

	if config.EnableDebugLogging {
		p.API.LogDebug("Message deletion allowed by policy",
			"user", user.Username,
			"channel", channel.Name,
		)
	}

	return post, ""
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
		channel, err = p.API.GetChannel(info.ChannelId)
		if err == nil {
			channelName = channel.Name
			channelHeader = channel.Header
		}
	}

	// Read file data for policy check (and to compute hash)
	fileData, err := io.ReadAll(file)
	if err != nil {
		p.API.LogError("Failed to read file", "error", err.Error())
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
	_, err = io.Copy(output, bytes.NewReader(fileData))
	if err != nil {
		p.API.LogError("Failed to write file", "error", err.Error())
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

// checkGenericPolicy checks a generic policy with any request structure
func (p *Plugin) checkGenericPolicy(req map[string]interface{}, policyPath string) (bool, string) {
	config := p.getConfiguration()

	// Validate configuration
	if config.DirektivURL == "" || config.DirektivNamespace == "" {
		p.API.LogError("Direktiv not configured - denying by default (fail-secure)")
		return false, "Policy service not configured"
	}

	// Build Direktiv URL
	endpoint := fmt.Sprintf("%s/ns/%s/policy/v1/%s",
		strings.TrimSuffix(config.DirektivURL, "/"),
		config.DirektivNamespace,
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
	if config.DirektivAPIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+config.DirektivAPIKey)
	}

	// Send request
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		p.API.LogError("Failed to contact Direktiv", "error", err.Error(), "endpoint", endpoint)
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
