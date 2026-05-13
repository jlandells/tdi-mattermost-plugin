package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	saml2 "github.com/mattermost/gosaml2"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

const (
	botUsername    = "tdi-policy-bot"
	botDisplayName = "TDI Policy Bot"
	botDescription = "Posts TDI policy enforcement notices."

	correlationIDHeader = "X-Correlation-ID"
)

// Plugin implements the interface expected by the Mattermost server to communicate between the server and plugin processes.
type Plugin struct {
	plugin.MattermostPlugin
	configuration     *configuration
	configurationLock sync.RWMutex
	httpClient        *http.Client
	router            *http.ServeMux
	botUserIDValue    string
	botUserIDLock     sync.RWMutex
}

// FileAttachmentInfo describes an attached file for policy decisions
type FileAttachmentInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	MimeType  string `json:"mime_type"`
	Extension string `json:"extension"` // e.g. ".pdf", ".exe"
}

// PolicyRequest represents a request sent to TDI for policy decision
type PolicyRequest struct {
	UserID          string                 `json:"user_id"`
	Username        string                 `json:"username"`
	Email           string                 `json:"email"`
	ChannelID       string                 `json:"channel_id"`
	ChannelName     string                 `json:"channel_name"`
	TeamID          string                 `json:"team_id"`
	TeamName        string                 `json:"team_name"`
	Message         string                 `json:"message,omitempty"`
	UserAttributes  map[string]interface{} `json:"user_attributes"`
	ChannelHeader   string                 `json:"channel_header,omitempty"`
	Action          string                 `json:"action"` // "message" or "channel_join"
	FileIds         []string               `json:"file_ids,omitempty"`
	FileAttachments []FileAttachmentInfo   `json:"file_attachments,omitempty"`
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

	// Initialize HTTP router for classify-channel and webapp API
	p.router = http.NewServeMux()
	p.router.HandleFunc("/classify-channel", p.handleClassifyChannel)
	p.router.HandleFunc("/api/policies", p.handleAPIPolicies)
	p.router.HandleFunc("/api/classify", p.handleAPIClassify)

	if err := p.ensureBotUser(); err != nil {
		p.logError("Failed to ensure plugin bot user; policy notices will be logged but not sent as DMs", "error", err.Error())
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
		UserID:          user.Id,
		Username:        user.Username,
		Email:           user.Email,
		ChannelID:       channel.Id,
		ChannelName:     channel.Name,
		TeamID:          channel.TeamId,
		TeamName:        teamName,
		Message:         post.Message,
		UserAttributes:  p.extractUserAttributes(user),
		ChannelHeader:   channel.Header,
		Action:          "message",
		FileIds:         post.FileIds,
		FileAttachments: p.buildFileAttachments(post.FileIds),
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
	policyPath := policyType
	switch policyType {
	case "message":
		policyPath = "message/check"
	case "channel_join":
		policyPath = "channel/join"
	}

	return p.checkTDIPolicy(req, policyPath)
}

// extractUserAttributes extracts user attributes from various sources for TDI policy evaluation.
//   - Basic fields (username, email, roles, etc.) are always included.
//   - Built-in User fields (first_name, last_name, nickname, position) are included so SAML-mapped
//     attributes flow through (SAML populates these, not user.Props).
//   - Custom profile attributes (Mattermost Enterprise v10.10+, e.g. SecurityClearance, Nationality)
//     are fetched via SearchPropertyValues / SearchPropertyFields and included by field name.
//     These sync from SAML/LDAP when linked in System Console > System properties.
//   - UserAttributeMapping maps policy keys to Mattermost/LDAP fields. Supports:
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

// buildFileAttachments fetches FileInfo for each file ID and returns metadata for policy evaluation.
// Extension is normalized to include a leading dot (e.g. ".pdf") for consistency with file-upload-policy.
func (p *Plugin) buildFileAttachments(fileIds []string) []FileAttachmentInfo {
	if len(fileIds) == 0 {
		return nil
	}
	out := make([]FileAttachmentInfo, 0, len(fileIds))
	for _, id := range fileIds {
		if id == "" {
			continue
		}
		info, appErr := p.API.GetFileInfo(id)
		if appErr != nil || info == nil {
			out = append(out, FileAttachmentInfo{ID: id})
			continue
		}
		ext := info.Extension
		if ext != "" && ext[0] != '.' {
			ext = "." + ext
		}
		out = append(out, FileAttachmentInfo{
			ID:        info.Id,
			Name:      info.Name,
			Size:      info.Size,
			MimeType:  info.MimeType,
			Extension: ext,
		})
	}
	return out
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
	botID := p.botUserID()
	if botID == "" {
		p.logError("Skipping denial DM because plugin bot user is not available", "user_id", userID, "channel", channelName)
		return
	}

	// Get or create DM channel with user
	dmChannel, err := p.API.GetDirectChannel(userID, botID)
	if err != nil {
		p.API.LogError("Failed to get DM channel", "error", err.Error())
		return
	}

	message := fmt.Sprintf("You were removed from channel **%s** because: %s", channelName, reason)

	post := &model.Post{
		UserId:    botID,
		ChannelId: dmChannel.Id,
		Message:   message,
	}

	if _, err := p.API.CreatePost(post); err != nil {
		p.API.LogError("Failed to send denial message", "error", err.Error())
	}
}

// ensureBotUser creates or updates the plugin bot used for policy notices.
func (p *Plugin) ensureBotUser() error {
	if p.API == nil {
		return fmt.Errorf("plugin API is not available")
	}

	botID, err := p.API.EnsureBotUser(&model.Bot{
		Username:    botUsername,
		DisplayName: botDisplayName,
		Description: botDescription,
	})
	if err != nil {
		p.setBotUserID("")
		return err
	}
	if botID == "" {
		p.setBotUserID("")
		return fmt.Errorf("Mattermost returned empty bot user ID")
	}

	p.setBotUserID(botID)
	return nil
}

func (p *Plugin) setBotUserID(botID string) {
	p.botUserIDLock.Lock()
	defer p.botUserIDLock.Unlock()
	p.botUserIDValue = botID
}

// botUserID returns the cached plugin bot user ID.
func (p *Plugin) botUserID() string {
	p.botUserIDLock.RLock()
	defer p.botUserIDLock.RUnlock()
	return p.botUserIDValue
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

	spooledFile, fileHash, readErr := p.spoolAndHashUpload(file, config.maxFileInspectionBytes())
	if readErr != nil {
		p.API.LogError("Failed to spool uploaded file", "error", readErr.Error())
		return nil, readErr.Error()
	}
	defer func() {
		name := spooledFile.Name()
		_ = spooledFile.Close()
		_ = os.Remove(name)
	}()

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

	if _, seekErr := spooledFile.Seek(0, io.SeekStart); seekErr != nil {
		p.API.LogError("Failed to rewind uploaded file", "error", seekErr.Error())
		return nil, "Failed to process file"
	}

	// Write file to output
	_, writeErr := io.Copy(output, spooledFile)
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

func (p *Plugin) spoolAndHashUpload(file io.Reader, maxBytes int64) (*os.File, string, error) {
	if maxBytes <= 0 {
		maxBytes = defaultMaxFileInspectionBytes
	}

	tmpFile, err := os.CreateTemp("", "mattermost-policy-upload-*")
	if err != nil {
		return nil, "", fmt.Errorf("Failed to process file")
	}

	hasher := sha256.New()
	limitedReader := &io.LimitedReader{R: file, N: maxBytes + 1}
	written, copyErr := io.Copy(io.MultiWriter(tmpFile, hasher), limitedReader)
	if copyErr != nil {
		name := tmpFile.Name()
		_ = tmpFile.Close()
		_ = os.Remove(name)
		return nil, "", fmt.Errorf("Failed to process file")
	}

	if written > maxBytes {
		name := tmpFile.Name()
		_ = tmpFile.Close()
		_ = os.Remove(name)
		return nil, "", fmt.Errorf("File exceeds maximum inspection size of %d bytes", maxBytes)
	}

	return tmpFile, fmt.Sprintf("%x", hasher.Sum(nil)), nil
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
		// if strings.Contains(strings.ToLower(channel.Name), "secret") {
		// 	channel.Header = "CLEARANCE_REQUIRED=SECRET"
		// 	p.API.UpdateChannel(channel)
		// }

		// Notify creator
		p.sendChannelCreationNotice(user.Id, channel.Name, reason)
		return
	}

	// Auto-classification logic
	// classification := p.classifyChannelByName(channel.Name)
	// if classification != "" {
	// 	channel.Header = fmt.Sprintf("CLEARANCE_REQUIRED=%s", classification)
	// 	p.API.UpdateChannel(channel)

	// 	if config.EnableDebugLogging {
	// 		p.API.LogDebug("Auto-classified channel",
	// 			"channel", channel.Name,
	// 			"classification", classification,
	// 		)
	// 	}
	// }
}

// // classifyChannelByName determines classification based on channel name
// func (p *Plugin) classifyChannelByName(name string) string {
// 	nameLower := strings.ToLower(name)

// 	if strings.Contains(nameLower, "ts-") || strings.Contains(nameLower, "topsecret") {
// 		return "TOP SECRET"
// 	}
// 	if strings.Contains(nameLower, "secret") || strings.Contains(nameLower, "classified") {
// 		return "SECRET"
// 	}
// 	if strings.Contains(nameLower, "confidential") {
// 		return "CONFIDENTIAL"
// 	}

// 	return ""
// }

// sendChannelCreationNotice sends a DM about channel creation policy
func (p *Plugin) sendChannelCreationNotice(userID, channelName, notice string) {
	botID := p.botUserID()
	if botID == "" {
		p.logError("Skipping channel creation notice because plugin bot user is not available", "user_id", userID, "channel", channelName)
		return
	}

	dmChannel, err := p.API.GetDirectChannel(userID, botID)
	if err != nil {
		p.API.LogError("Failed to get DM channel", "error", err.Error())
		return
	}

	message := fmt.Sprintf("**Channel Creation Notice**\n\nChannel: **%s**\n\n%s", channelName, notice)

	post := &model.Post{
		UserId:    botID,
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
		"user_id":      channelMember.UserId,
		"channel_id":   channelMember.ChannelId,
		"channel_name": channelName,
		"action":       "channel_leave",
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
		"user_id":    userID,
		"post_id":    pushNotification.PostId,
		"channel_id": pushNotification.ChannelId,
		"server_id":  pushNotification.ServerId,
		"message":    pushNotification.Message,
		"action":     "push_notification",
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
	return p.checkTDIPolicy(req, policyPath)
}

// checkTDIPolicy sends a policy decision request to TDI and interprets the
// gateway/workflow response. It fails secure for malformed config, transport
// errors, non-200 responses, invalid JSON, and unknown policy actions.
func (p *Plugin) checkTDIPolicy(req interface{}, policyPath string) (bool, string) {
	config := p.getConfiguration()
	correlationID := newCorrelationID()

	// Validate configuration
	if err := p.validateConfiguration(config); err != nil {
		p.logError("TDI not configured - denying by default (fail-secure)", "correlation_id", correlationID, "error", err.Error())
		return false, "Policy service not configured"
	}

	// Build TDI URL
	endpoint := fmt.Sprintf("%s/ns/%s/policy/v1/%s",
		strings.TrimSuffix(config.TDIURL, "/"),
		url.PathEscape(config.TDINamespace),
		strings.TrimPrefix(policyPath, "/"))

	// Marshal request
	jsonData, err := json.Marshal(req)
	if err != nil {
		p.logError("Failed to marshal policy request", "correlation_id", correlationID, "error", err.Error())
		return false, "Internal error processing policy request"
	}

	if config.EnableDebugLogging {
		p.logDebug("Sending policy request", "correlation_id", correlationID, "endpoint", endpoint, "policy_path", policyPath, "payload", redactedPolicyPayload(jsonData))
	}

	// Create HTTP request
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.PolicyTimeout)*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		p.logError("Failed to create HTTP request", "correlation_id", correlationID, "error", err.Error())
		return false, "Internal error contacting policy service"
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set(correlationIDHeader, correlationID)
	if config.TDIAPIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+config.TDIAPIKey)
	}

	// Send request
	if p.httpClient == nil {
		p.httpClient = &http.Client{Timeout: time.Duration(config.PolicyTimeout) * time.Second}
	}
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		p.logError("Failed to contact TDI", "correlation_id", correlationID, "error", err.Error(), "endpoint", endpoint)
		return false, "Policy service unavailable"
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		p.logError("Failed to read policy response", "correlation_id", correlationID, "error", err.Error())
		return false, "Error reading policy response"
	}

	if config.EnableDebugLogging {
		p.logDebug("Policy response received", "correlation_id", correlationID, "status", resp.StatusCode, "body", redactedPolicyPayload(body))
	}

	// Check HTTP status
	if resp.StatusCode != http.StatusOK {
		p.logError("Policy service returned error", "correlation_id", correlationID, "status", resp.StatusCode, "body", redactedPolicyPayload(body))
		return false, "Policy service error"
	}

	// Parse response
	var policyResp PolicyResponse
	if err := json.Unmarshal(body, &policyResp); err != nil {
		p.logError("Failed to parse policy response", "correlation_id", correlationID, "error", err.Error())
		return false, "Invalid policy response"
	}

	// Check response
	switch policyResp.Action {
	case "continue":
		return true, ""
	case "reject":
		reason := "Access denied by policy"
		if reasonStr, ok := policyResp.Result["reason"].(string); ok && reasonStr != "" {
			reason = reasonStr
		}
		return false, reason
	default:
		p.logError("Policy service returned unknown action", "correlation_id", correlationID, "action", policyResp.Action)
		return false, "Invalid policy response"
	}
}

func (p *Plugin) logError(msg string, keyValuePairs ...interface{}) {
	if p.API != nil {
		p.API.LogError(msg, keyValuePairs...)
	}
}

func (p *Plugin) logDebug(msg string, keyValuePairs ...interface{}) {
	if p.API != nil {
		p.API.LogDebug(msg, keyValuePairs...)
	}
}

func newCorrelationID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("policy-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func redactedPolicyPayload(raw []byte) string {
	var payload interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Sprintf("<non-json payload: %d bytes>", len(raw))
	}

	redacted := redactValue(payload)
	out, err := json.Marshal(redacted)
	if err != nil {
		return "<redaction failed>"
	}
	return string(out)
}

func redactValue(value interface{}) interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(v))
		for key, val := range v {
			if isSensitivePolicyField(key) {
				out[key] = redactionSummary(val)
				continue
			}
			out[key] = redactValue(val)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(v))
		for i, item := range v {
			out[i] = redactValue(item)
		}
		return out
	default:
		return v
	}
}

func isSensitivePolicyField(key string) bool {
	switch strings.ToLower(key) {
	case "message",
		"old_message",
		"new_message",
		"user_attributes",
		"channel_header",
		"email",
		"file_hash",
		"mattermost_api_token",
		"tdi_api_key",
		"authorization",
		"reason":
		return true
	default:
		return false
	}
}

func redactionSummary(value interface{}) string {
	switch v := value.(type) {
	case string:
		return fmt.Sprintf("<redacted:%d chars>", len(v))
	case []interface{}:
		return fmt.Sprintf("<redacted:%d items>", len(v))
	case map[string]interface{}:
		return fmt.Sprintf("<redacted:%d fields>", len(v))
	case nil:
		return "<redacted:null>"
	default:
		return "<redacted>"
	}
}

// ============================================================================
// ServeHTTP - classify-channel endpoint for access control policy assignment
// ============================================================================

// ServeHTTP allows the plugin to implement the http.Handler interface.
// Requests to /plugins/{id}/* are routed here.
func (p *Plugin) ServeHTTP(_ *plugin.Context, w http.ResponseWriter, r *http.Request) {
	p.router.ServeHTTP(w, r)
}

// AccessControlPolicy and related types for Mattermost REST API (not in plugin API)
type accessControlPolicy struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Active   bool   `json:"active"`
	CreateAt int64  `json:"create_at"`
}

type accessControlPoliciesResponse struct {
	Policies []*accessControlPolicy `json:"policies"`
	Total    int64                  `json:"total"`
}

// policyChannelsResponse holds the response from GET .../access_control_policies/{id}/resources/channels
type policyChannelsResponse struct {
	Channels []struct {
		ID string `json:"id"`
	} `json:"channels"`
	TotalCount int64 `json:"total_count"`
}

// isChannelAdmin returns true if the user is a channel admin (or system admin) for the channel
func (p *Plugin) isChannelAdmin(channelID, userID string) bool {
	user, err := p.API.GetUser(userID)
	if err != nil || user == nil {
		return false
	}
	if user.IsSystemAdmin() {
		return true
	}
	member, err := p.API.GetChannelMember(channelID, userID)
	if err != nil || member == nil {
		return false
	}
	return strings.Contains(member.Roles, "channel_admin")
}

// handleAPIPolicies returns JSON list of access control policies (for webapp)
func (p *Plugin) handleAPIPolicies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID := r.Header.Get("Mattermost-User-Id")
	if userID == "" {
		p.serveJSONError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}
	channelID := r.URL.Query().Get("channel_id")
	if channelID == "" {
		p.serveJSONError(w, http.StatusBadRequest, "channel_id required")
		return
	}
	if !p.isChannelAdmin(channelID, userID) {
		p.serveJSONError(w, http.StatusForbidden, "Only channel admins can classify channels")
		return
	}
	config := p.getConfiguration()
	if config.MattermostAPIToken == "" {
		p.serveJSONError(w, http.StatusServiceUnavailable, "Channel classification not configured")
		return
	}
	policies, err := p.fetchAccessControlPolicies()
	if err != nil {
		p.API.LogError("Failed to fetch policies for API", "error", err.Error())
		p.serveJSONError(w, http.StatusInternalServerError, "Failed to load policies")
		return
	}
	currentPolicy, _ := p.fetchChannelAssignedPolicy(channelID)
	payload := map[string]interface{}{"policies": policies}
	if currentPolicy != nil {
		payload["current_policy"] = map[string]interface{}{"id": currentPolicy.ID, "name": currentPolicy.Name}
	} else {
		payload["current_policy"] = nil
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

// handleAPIClassify assigns a policy to a channel (for webapp)
func (p *Plugin) handleAPIClassify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID := r.Header.Get("Mattermost-User-Id")
	if userID == "" {
		p.serveJSONError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}
	config := p.getConfiguration()
	if config.MattermostAPIToken == "" {
		p.serveJSONError(w, http.StatusServiceUnavailable, "Channel classification not configured")
		return
	}
	var req struct {
		ChannelID string `json:"channel_id"`
		PolicyID  string `json:"policy_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		p.serveJSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.ChannelID == "" || req.PolicyID == "" {
		p.serveJSONError(w, http.StatusBadRequest, "channel_id and policy_id required")
		return
	}
	channel, err := p.API.GetChannel(req.ChannelID)
	if err != nil || channel == nil {
		p.serveJSONError(w, http.StatusNotFound, "Channel not found")
		return
	}
	if !p.isChannelAdmin(req.ChannelID, userID) {
		p.serveJSONError(w, http.StatusForbidden, "Only channel admins can classify channels")
		return
	}
	if err := p.assignPolicyToChannel(req.PolicyID, req.ChannelID); err != nil {
		p.API.LogError("Failed to assign policy via API", "error", err.Error(), "channel_id", req.ChannelID, "policy_id", req.PolicyID)
		p.serveJSONError(w, http.StatusInternalServerError, "Failed to assign policy")
		return
	}
	user, _ := p.API.GetUser(userID)
	username := "A user"
	if user != nil {
		username = user.Username
	}
	post := &model.Post{
		UserId:    userID,
		ChannelId: req.ChannelID,
		Message:   fmt.Sprintf("Channel **%s** has been classified. Users can now join based on the assigned access control policy.", channel.Name),
	}
	if _, err := p.API.CreatePost(post); err != nil {
		p.API.LogError("Failed to post classification confirmation", "error", err.Error())
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":      true,
		"message":      "Channel classified successfully",
		"channel_name": channel.Name,
		"username":     username,
	})
}

func (p *Plugin) serveJSONError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": msg})
}

// handleClassifyChannel serves GET (form) and handles POST (submit)
func (p *Plugin) handleClassifyChannel(w http.ResponseWriter, r *http.Request) {
	config := p.getConfiguration()
	if config.MattermostAPIToken == "" {
		p.serveClassifyError(w, "Channel classification is not configured. Ask your administrator to set the Mattermost API Token.")
		return
	}

	userID := r.Header.Get("Mattermost-User-Id")
	if userID == "" {
		p.serveClassifyError(w, "You must be logged in to classify a channel.")
		return
	}

	switch r.Method {
	case http.MethodGet:
		p.handleClassifyChannelGET(w, r, userID)
	case http.MethodPost:
		p.handleClassifyChannelPOST(w, r, userID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (p *Plugin) handleClassifyChannelGET(w http.ResponseWriter, r *http.Request, userID string) {
	channelID := r.URL.Query().Get("channel_id")
	if channelID == "" {
		p.serveClassifyError(w, "Missing channel_id parameter.")
		return
	}

	channel, err := p.API.GetChannel(channelID)
	if err != nil || channel == nil {
		p.serveClassifyError(w, "Channel not found.")
		return
	}

	if !p.isChannelAdmin(channelID, userID) {
		p.serveClassifyError(w, "Only channel admins can classify channels.")
		return
	}

	// Fetch parent policies via REST
	policies, fetchErr := p.fetchAccessControlPolicies()
	if fetchErr != nil {
		p.API.LogError("Failed to fetch access control policies", "error", fetchErr.Error())
		p.serveClassifyError(w, "Failed to load policies. Please try again later.")
		return
	}

	p.serveClassifyForm(w, channelID, channel.Name, policies)
}

func (p *Plugin) handleClassifyChannelPOST(w http.ResponseWriter, r *http.Request, userID string) {
	if err := r.ParseForm(); err != nil {
		p.serveClassifyError(w, "Invalid form data.")
		return
	}

	channelID := r.FormValue("channel_id")
	policyID := r.FormValue("policy_id")
	if channelID == "" || policyID == "" {
		p.serveClassifyError(w, "Missing channel_id or policy_id.")
		return
	}

	channel, err := p.API.GetChannel(channelID)
	if err != nil || channel == nil {
		p.serveClassifyError(w, "Channel not found.")
		return
	}

	if !p.isChannelAdmin(channelID, userID) {
		p.serveClassifyError(w, "Only channel admins can classify channels.")
		return
	}

	if err := p.assignPolicyToChannel(policyID, channelID); err != nil {
		p.API.LogError("Failed to assign policy to channel", "error", err.Error(), "channel_id", channelID, "policy_id", policyID)
		p.serveClassifyError(w, "Failed to assign policy. Please try again.")
		return
	}

	// Post confirmation in the channel
	user, _ := p.API.GetUser(userID)
	username := "A user"
	if user != nil {
		username = user.Username
	}
	post := &model.Post{
		UserId:    userID,
		ChannelId: channelID,
		Message:   fmt.Sprintf("Channel **%s** has been classified. Users can now join based on the assigned access control policy.", channel.Name),
	}
	if _, err := p.API.CreatePost(post); err != nil {
		p.API.LogError("Failed to post classification confirmation", "error", err.Error())
	}

	p.serveClassifySuccess(w, channel.Name, username)
}

func (p *Plugin) fetchAccessControlPolicies() ([]*accessControlPolicy, error) {
	config := p.getConfiguration()
	siteURL := p.getSiteURL()
	if siteURL == "" {
		return nil, fmt.Errorf("site URL not configured")
	}

	searchURL := strings.TrimSuffix(siteURL, "/") + "/api/v4/access_control_policies/search"
	body := bytes.NewReader([]byte(`{"type":"parent"}`))

	req, err := http.NewRequest("POST", searchURL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.MattermostAPIToken)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result accessControlPoliciesResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	// Include all parent policies (active and inactive) so admins can assign any of them
	out := make([]*accessControlPolicy, 0, len(result.Policies))
	for _, pol := range result.Policies {
		if pol != nil && pol.Type == "parent" {
			out = append(out, pol)
		}
	}
	return out, nil
}

// fetchChannelAssignedPolicy returns the parent policy currently assigned to the channel, or nil.
func (p *Plugin) fetchChannelAssignedPolicy(channelID string) (*accessControlPolicy, error) {
	policies, err := p.fetchAccessControlPolicies()
	if err != nil {
		return nil, err
	}
	config := p.getConfiguration()
	siteURL := p.getSiteURL()
	if siteURL == "" {
		return nil, nil
	}
	baseURL := strings.TrimSuffix(siteURL, "/")
	for _, pol := range policies {
		if pol == nil {
			continue
		}
		after := ""
		const limit = 100
		for {
			urlStr := baseURL + "/api/v4/access_control_policies/" + url.PathEscape(pol.ID) + "/resources/channels?limit=" + fmt.Sprintf("%d", limit)
			if after != "" {
				urlStr += "&after=" + url.QueryEscape(after)
			}
			req, err := http.NewRequest("GET", urlStr, nil)
			if err != nil {
				return nil, err
			}
			req.Header.Set("Authorization", "Bearer "+config.MattermostAPIToken)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			req = req.WithContext(ctx)
			resp, err := p.httpClient.Do(req)
			cancel()
			if err != nil {
				return nil, err
			}
			respBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				break // skip this policy
			}
			var chResp policyChannelsResponse
			if json.Unmarshal(respBody, &chResp) != nil {
				break
			}
			for _, ch := range chResp.Channels {
				if ch.ID == channelID {
					return pol, nil
				}
			}
			if int64(len(chResp.Channels)) < limit || len(chResp.Channels) == 0 {
				break
			}
			after = chResp.Channels[len(chResp.Channels)-1].ID
		}
	}
	return nil, nil
}

func (p *Plugin) unassignPolicyFromChannel(policyID, channelID string) error {
	config := p.getConfiguration()
	siteURL := p.getSiteURL()
	if siteURL == "" {
		return fmt.Errorf("site URL not configured")
	}
	unassignURL := strings.TrimSuffix(siteURL, "/") + "/api/v4/access_control_policies/" + url.PathEscape(policyID) + "/unassign"
	payload := map[string][]string{"channel_ids": {channelID}}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("DELETE", unassignURL, bytes.NewReader(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.MattermostAPIToken)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req = req.WithContext(ctx)
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	// 200, 204, or 404 (not assigned) are all acceptable
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("unassign API returned %d", resp.StatusCode)
	}
	return nil
}

func (p *Plugin) assignPolicyToChannel(policyID, channelID string) error {
	config := p.getConfiguration()
	siteURL := p.getSiteURL()
	if siteURL == "" {
		return fmt.Errorf("site URL not configured")
	}

	// Unassign all parent policies from the channel before assigning the new one (removes old classification)
	policies, err := p.fetchAccessControlPolicies()
	if err == nil {
		for _, pol := range policies {
			if pol != nil {
				_ = p.unassignPolicyFromChannel(pol.ID, channelID)
			}
		}
	}

	assignURL := strings.TrimSuffix(siteURL, "/") + "/api/v4/access_control_policies/" + url.PathEscape(policyID) + "/assign"
	payload := map[string][]string{"channel_ids": {channelID}}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", assignURL, bytes.NewReader(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.MattermostAPIToken)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("assign API returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func (p *Plugin) getSiteURL() string {
	cfg := p.API.GetConfig()
	if cfg == nil || cfg.ServiceSettings.SiteURL == nil {
		return ""
	}
	return strings.TrimSuffix(*cfg.ServiceSettings.SiteURL, "/")
}

func (p *Plugin) serveClassifyForm(w http.ResponseWriter, channelID, channelName string, policies []*accessControlPolicy) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	opts := ""
	for _, pol := range policies {
		tag := " (inactive)"
		if pol.Active {
			tag = " (active)"
		}
		opts += fmt.Sprintf(`<option value="%s">%s</option>`, html.EscapeString(pol.ID), html.EscapeString(pol.Name+tag))
	}
	if opts == "" {
		opts = `<option value="">No policies available</option>`
	}

	page := `<!DOCTYPE html>
<html>
<head><meta charset="utf-8"><title>Classify Channel</title></head>
<body style="font-family:sans-serif;max-width:480px;margin:2rem auto;padding:1rem;">
  <h2>Classify Channel: ` + html.EscapeString(channelName) + `</h2>
  <p>Select an access control policy to apply to this channel. Until classified, users may not be able to join.</p>
  <form id="classify-form">
    <input type="hidden" name="channel_id" value="` + html.EscapeString(channelID) + `" />
    <label for="policy_id">Policy:</label><br/>
    <select name="policy_id" id="policy_id" required style="width:100%;padding:0.5rem;margin:0.5rem 0;">` + opts + `</select><br/>
    <button type="submit" id="submit-btn" style="margin-top:1rem;padding:0.5rem 1rem;">Apply Policy</button>
    <span id="submit-msg" style="margin-left:0.5rem;color:#666;"></span>
  </form>
  <script>
    document.getElementById('classify-form').addEventListener('submit', function(e) {
      e.preventDefault();
      var btn = document.getElementById('submit-btn');
      var msg = document.getElementById('submit-msg');
      btn.disabled = true;
      msg.textContent = 'Applying...';
      var form = e.target;
      var formData = new FormData(form);
      var csrf = document.cookie.split(';').reduce(function(acc, c) {
        var parts = c.trim().split('=');
        if (parts[0] === 'MMCSRF') acc = decodeURIComponent(parts.slice(1).join('='));
        return acc;
      }, '');
      fetch(window.location.pathname + window.location.search, {
        method: 'POST',
        body: new URLSearchParams(formData),
        headers: {
          'Content-Type': 'application/x-www-form-urlencoded',
          'X-CSRF-Token': csrf
        },
        credentials: 'same-origin'
      }).then(function(r) {
        return r.text().then(function(html) {
          document.body.innerHTML = html;
        });
      }).catch(function(err) {
        btn.disabled = false;
        msg.textContent = '';
        alert('Request failed: ' + err.message);
      });
    });
  </script>
</body>
</html>`
	w.Write([]byte(page))
}

func (p *Plugin) serveClassifyError(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusBadRequest)
	page := `<!DOCTYPE html>
<html>
<head><meta charset="utf-8"><title>Error</title></head>
<body style="font-family:sans-serif;max-width:480px;margin:2rem auto;padding:1rem;">
  <p style="color:#c00;">` + html.EscapeString(msg) + `</p>
  <p><a href="javascript:history.back()">Go back</a></p>
</body>
</html>`
	w.Write([]byte(page))
}

func (p *Plugin) serveClassifySuccess(w http.ResponseWriter, channelName, username string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	page := `<!DOCTYPE html>
<html>
<head><meta charset="utf-8"><title>Channel Classified</title></head>
<body style="font-family:sans-serif;max-width:480px;margin:2rem auto;padding:1rem;">
  <h2>Channel Classified</h2>
  <p>Channel <strong>` + html.EscapeString(channelName) + `</strong> has been successfully classified by <strong>` + html.EscapeString(username) + `</strong>. A confirmation message was posted in the channel.</p>
  <p>You can close this window and return to Mattermost.</p>
</body>
</html>`
	w.Write([]byte(page))
}

func main() {
	plugin.ClientMain(&Plugin{})
}
