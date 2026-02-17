# Mattermost Plugin Hooks - Complete Reference

This document lists all the hooks available in Mattermost plugins that can be used to integrate with TDI for policy enforcement.

## Server Hooks Available

### 🔴 **Already Implemented**

#### 1. MessageWillBePosted
**Status:** ✅ Implemented in our plugin

**Use Case:** Approve/deny messages before they're posted

**Policy Examples:**
- Clearance-based channel restrictions
- Content filtering/DLP
- Time-based posting restrictions
- Department-specific channels

#### 2. UserHasJoinedChannel
**Status:** ✅ Implemented in our plugin

**Use Case:** React to users joining channels (can remove them)

**Policy Examples:**
- Clearance-based channel access
- Department restrictions
- Maximum channel membership limits
- Contractor exclusions

---

### 🟢 **Can Be Added - High Value**

#### 3. MessageWillBeUpdated
**Signature:** `MessageWillBeUpdated(c *plugin.Context, newPost, oldPost *model.Post) (*model.Post, string)`

**Use Case:** Control message editing

**Policy Examples:**
- Prevent editing of classified messages
- Require approval for message edits
- Audit all message changes
- Lock messages after time period

**Implementation:**
```go
func (p *Plugin) MessageWillBeUpdated(c *plugin.Context, newPost, oldPost *model.Post) (*model.Post, string) {
    // Check if edit is allowed
    allowed, reason := p.checkMessageEditPolicy(newPost, oldPost)
    if !allowed {
        return oldPost, reason  // Return original, deny edit
    }
    return newPost, ""
}
```

**TDI Workflow:** `message-edit-policy.yaml`

#### 4. MessageWillBeDeleted
**Signature:** `MessageWillBeDeleted(c *plugin.Context, post *model.Post) (*model.Post, string)`

**Use Case:** Control message deletion

**Policy Examples:**
- Prevent deletion of audit trail messages
- Require manager approval for deletions
- Preserve classified message history
- Time-based deletion restrictions

**Implementation:**
```go
func (p *Plugin) MessageWillBeDeleted(c *plugin.Context, post *model.Post) (*model.Post, string) {
    allowed, reason := p.checkMessageDeletePolicy(post)
    if !allowed {
        return nil, reason  // Deny deletion
    }
    return post, ""
}
```

#### 5. FileWillBeUploaded
**Signature:** `FileWillBeUploaded(c *plugin.Context, info *model.FileInfo, file io.Reader, output io.Writer) (*model.FileInfo, string)`

**Use Case:** Control file uploads

**Policy Examples:**
- Block certain file types in classified channels
- File size restrictions per clearance level
- Malware scanning integration
- DLP for file content
- Contractor upload restrictions

**Implementation:**
```go
func (p *Plugin) FileWillBeUploaded(c *plugin.Context, info *model.FileInfo, file io.Reader, output io.Writer) (*model.FileInfo, string) {
    // Read file for scanning
    buf := new(bytes.Buffer)
    tee := io.TeeReader(file, buf)
    
    // Check policy
    allowed, reason := p.checkFileUploadPolicy(info, buf.Bytes())
    if !allowed {
        return nil, reason
    }
    
    // Copy to output
    io.Copy(output, buf)
    return info, ""
}
```

**TDI Workflow:** `file-upload-policy.yaml`

#### 6. UserWillLogIn
**Signature:** `UserWillLogIn(c *plugin.Context, user *model.User) string`

**Use Case:** Control user login

**Policy Examples:**
- Time-based access (business hours only)
- Location-based restrictions
- Compromised account detection
- Force MFA for certain clearance levels

**Implementation:**
```go
func (p *Plugin) UserWillLogIn(c *plugin.Context, user *model.User) string {
    allowed, reason := p.checkLoginPolicy(user)
    if !allowed {
        return reason  // Deny login
    }
    return ""
}
```

#### 7. ChannelHasBeenCreated
**Signature:** `ChannelHasBeenCreated(c *plugin.Context, channel *model.Channel)`

**Use Case:** React to channel creation

**Policy Examples:**
- Auto-classify channels based on name
- Enforce channel naming conventions
- Set default permissions
- Notify security team of classified channels

**Implementation:**
```go
func (p *Plugin) ChannelHasBeenCreated(c *plugin.Context, channel *model.Channel) {
    // Auto-classify channel
    classification := p.classifyChannel(channel)
    
    // Update channel header with classification
    if classification != "UNCLASSIFIED" {
        channel.Header = fmt.Sprintf("CLEARANCE_REQUIRED=%s", classification)
        p.API.UpdateChannel(channel)
    }
}
```

#### 8. UserHasLeftChannel
**Signature:** `UserHasLeftChannel(c *plugin.Context, channelMember *model.ChannelMember, actor *model.User)`

**Use Case:** React to users leaving channels

**Policy Examples:**
- Log departures from classified channels
- Notify team of key personnel departure
- Revoke access to related resources

---

### 🟡 **Can Be Added - Medium Value**

#### 9. ReactionHasBeenAdded
**Signature:** `ReactionHasBeenAdded(c *plugin.Context, reaction *model.Reaction)`

**Use Case:** Control emoji reactions

**Policy Examples:**
- Restrict reactions in classified channels
- Approval workflow via reactions
- Prevent inappropriate reactions

#### 10. ReactionHasBeenRemoved
**Signature:** `ReactionHasBeenRemoved(c *plugin.Context, reaction *model.Reaction)`

**Use Case:** React to reaction removal

#### 11. UserHasBeenCreated
**Signature:** `UserHasBeenCreated(c *plugin.Context, user *model.User)`

**Use Case:** React to new user creation

**Policy Examples:**
- Auto-assign clearance attributes
- Trigger onboarding workflow
- Notify security team
- Set default channel memberships

#### 12. UserHasJoinedTeam
**Signature:** `UserHasJoinedTeam(c *plugin.Context, teamMember *model.TeamMember, actor *model.User)`

**Use Case:** Control team access

**Policy Examples:**
- Clearance-based team access
- Department restrictions
- Auto-join to classified channels

#### 13. UserHasLeftTeam
**Signature:** `UserHasLeftTeam(c *plugin.Context, teamMember *model.TeamMember, actor *model.User)`

**Use Case:** React to users leaving teams

#### 14. ChannelHasBeenDeleted
**Signature:** `ChannelHasBeenDeleted(c *plugin.Context, channel *model.Channel)`

**Use Case:** React to channel deletion

**Policy Examples:**
- Archive classified channel data
- Notify stakeholders
- Audit trail

---

### 🔵 **Can Be Added - Lower Priority**

#### 15. TeamHasBeenCreated
#### 16. RoleHasBeenUpdated
#### 17. BotHasBeenCreated
#### 18. StatusHasBeenUpdated
#### 19. PluginConfigurationWillBeSaved
#### 20. WebSocketMessageHasBeenPosted
#### 21. PreferencesHasBeenSaved
#### 22. PreferencesHasBeenDeleted

---

## Most Valuable Additions

Based on security and access control needs, here are the **top 5 hooks to implement next**:

### 1. 🥇 **FileWillBeUploaded** (Critical)
**Why:** File uploads are a major security risk
- DLP scanning
- Malware detection
- File type restrictions
- Size limits per clearance

### 2. 🥈 **MessageWillBeDeleted** (High)
**Why:** Prevent destruction of audit trails
- Preserve classified communications
- Enforce retention policies
- Compliance requirements

### 3. 🥉 **MessageWillBeUpdated** (High)
**Why:** Prevent tampering with classified messages
- Audit message changes
- Lock critical communications
- Detect unauthorized modifications

### 4. 🎯 **UserWillLogIn** (Medium-High)
**Why:** First line of defense
- Time-based access control
- Location restrictions
- Compromised account detection

### 5. 🎯 **ChannelHasBeenCreated** (Medium)
**Why:** Automated channel classification
- Enforce naming conventions
- Auto-set permissions
- Security team notifications

---

## Implementation Priority Matrix

```
High Security Impact + High Frequency = TOP PRIORITY

┌─────────────────────────────────────────────┐
│                                             │
│  High Frequency                             │
│  ┌─────────────────────────────────────┐   │
│  │ MessageWillBePosted      ✅         │   │
│  │ UserHasJoinedChannel     ✅         │   │
│  │ FileWillBeUploaded       🔴         │   │
│  │ MessageWillBeUpdated     🔴         │   │
│  └─────────────────────────────────────┘   │
│                                             │
│  Medium Frequency                           │
│  ┌─────────────────────────────────────┐   │
│  │ MessageWillBeDeleted     🟡         │   │
│  │ UserWillLogIn           🟡         │   │
│  │ ChannelHasBeenCreated   🟡         │   │
│  └─────────────────────────────────────┘   │
│                                             │
│  Low Frequency                              │
│  ┌─────────────────────────────────────┐   │
│  │ ReactionHasBeenAdded    🔵         │   │
│  │ UserHasBeenCreated      🔵         │   │
│  └─────────────────────────────────────┘   │
│                                             │
└─────────────────────────────────────────────┘
    Low Impact ────────────────▶ High Impact
```

---

## Implementation Example: File Upload Policy

### Plugin Hook (plugin/main.go)
```go
func (p *Plugin) FileWillBeUploaded(c *plugin.Context, info *model.FileInfo, file io.Reader, output io.Writer) (*model.FileInfo, string) {
    config := p.getConfiguration()
    
    if !config.EnableFileUploadPolicy {
        io.Copy(output, file)
        return info, ""
    }
    
    // Get user
    user, err := p.API.GetUser(info.CreatorId)
    if err != nil {
        return nil, "Failed to verify user"
    }
    
    // Get channel
    channel, err := p.API.GetChannel(info.ChannelId)
    if err != nil {
        return nil, "Failed to verify channel"
    }
    
    // Read file data
    fileData, err := io.ReadAll(file)
    if err != nil {
        return nil, "Failed to read file"
    }
    
    // Build policy request
    policyReq := FileUploadPolicyRequest{
        UserID:         user.Id,
        Username:       user.Username,
        ChannelID:      channel.Id,
        ChannelName:    channel.Name,
        FileName:       info.Name,
        FileSize:       info.Size,
        MimeType:       info.MimeType,
        FileExtension:  filepath.Ext(info.Name),
        UserAttributes: p.extractUserAttributes(user),
        FileHash:       fmt.Sprintf("%x", sha256.Sum256(fileData)),
    }
    
    // Check policy
    allowed, reason := p.checkFileUploadPolicy(policyReq)
    if !allowed {
        return nil, reason
    }
    
    // Write file to output
    _, err = io.Copy(output, bytes.NewReader(fileData))
    if err != nil {
        return nil, "Failed to process file"
    }
    
    return info, ""
}
```

### TDI Gateway (gateways/file-upload-policy.yaml)
```yaml
x-direktiv-api: endpoint/v2
x-direktiv-config:
  allow_anonymous: true
  path: /policy/v1/file/upload
  plugins:
    auth: []
    inbound:
      - type: request-convert
        configuration:
          omit_headers: false
          omit_queries: false
          omit_body: false
          omit_consumer: false 
    target:
      configuration:
        async: false
        flow: /workflows/file-upload-policy.yaml
      type: target-flow
post:
  summary: File upload policy check
  description: Validates whether a user can upload a file to a channel
  operationId: checkFileUploadPolicy
  requestBody:
    required: true
    content:
      application/json:
        schema:
          type: object
          properties:
            user_id:
              type: string
            filename:
              type: string
            file_size:
              type: integer
            mime_type:
              type: string
            channel_name:
              type: string
            user_attributes:
              type: object
```

### TDI Workflow (workflows/file-upload-policy.yaml)
```yaml
direktiv_api: workflow/v1

description: Evaluate file upload policy

states:
  - id: log-request
    type: noop
    log: |
      File upload check: user=jq(.body.username) file=jq(.body.filename) channel=jq(.body.channel_name)
    transform: jq(.)
    transition: evaluate-policy

  - id: evaluate-policy
    type: switch
    defaultTransition: allow-upload
    conditions:
      # Block executables in all channels
      - condition: |
          jq(
            [".exe", ".sh", ".bat", ".cmd", ".ps1"] as $blocked |
            $blocked | any(. as $ext | ($.body.file_extension | endswith($ext)))
          )
        transition: deny-executable
      
      # Block large files in classified channels
      - condition: |
          jq(
            (.body.channel_name | contains("classified")) and
            (.body.file_size > 10485760)  # 10MB
          )
        transition: deny-file-too-large
      
      # Check clearance for file types in classified channels
      - condition: |
          jq(
            (.body.channel_name | contains("secret")) and
            (.body.mime_type | startswith("application/")) and
            ((.body.user_attributes.clearance // "") != "SECRET")
          )
        transition: deny-insufficient-clearance

  - id: allow-upload
    type: noop
    transform: |
      jq({
        status: "success",
        action: "continue",
        result: {}
      })

  - id: deny-executable
    type: noop
    transform: |
      jq({
        status: "success",
        action: "reject",
        result: {
          reason: "Executable files are not allowed"
        }
      })

  - id: deny-file-too-large
    type: noop
    transform: |
      jq({
        status: "success",
        action: "reject",
        result: {
          reason: "Files larger than 10MB are not allowed in classified channels"
        }
      })

  - id: deny-insufficient-clearance
    type: noop
    transform: |
      jq({
        status: "success",
        action: "reject",
        result: {
          reason: "SECRET clearance required to upload files to this channel"
        }
      })
```

---

## Extending the Plugin

### Step 1: Add Hook to Plugin

```go
// In plugin/main.go
func (p *Plugin) MessageWillBeUpdated(c *plugin.Context, newPost, oldPost *model.Post) (*model.Post, string) {
    config := p.getConfiguration()
    
    if !config.EnableMessageEditPolicy {
        return newPost, ""
    }
    
    // Build policy request
    policyReq := MessageEditPolicyRequest{
        UserID:      newPost.UserId,
        ChannelID:   newPost.ChannelId,
        OldMessage:  oldPost.Message,
        NewMessage:  newPost.Message,
        EditTime:    time.Since(time.Unix(oldPost.CreateAt/1000, 0)),
    }
    
    // Check policy
    allowed, reason := p.checkMessageEditPolicy(policyReq)
    if !allowed {
        return oldPost, reason  // Return original
    }
    
    return newPost, ""
}
```

### Step 2: Add Configuration Option

```json
// In plugin/plugin.json
{
  "key": "EnableMessageEditPolicy",
  "display_name": "Enable Message Edit Policy Checks",
  "type": "bool",
  "help_text": "When enabled, message edits will be checked against policy",
  "default": false
}
```

### Step 3: Create TDI Gateway

Create `gateways/message-edit-policy.yaml` following the same pattern as existing gateways.

### Step 4: Create TDI Workflow

Create `workflows/message-edit-policy.yaml` with your policy logic.

---

## Summary: What Can Be Controlled

| Hook | Policy Use Case | Priority | Effort |
|------|----------------|----------|--------|
| ✅ MessageWillBePosted | Message approval | **Implemented** | - |
| ✅ UserHasJoinedChannel | Channel access | **Implemented** | - |
| 🔴 FileWillBeUploaded | File restrictions | Critical | Medium |
| 🔴 MessageWillBeDeleted | Audit preservation | High | Low |
| 🔴 MessageWillBeUpdated | Edit control | High | Low |
| 🟡 UserWillLogIn | Login restrictions | Medium | Low |
| 🟡 ChannelHasBeenCreated | Auto-classification | Medium | Low |
| 🔵 ReactionHasBeenAdded | Reaction control | Low | Low |
| 🔵 UserHasBeenCreated | User onboarding | Low | Low |

**Total Available Hooks:** 22+  
**Currently Implemented:** 2  
**High Value Candidates:** 5  
**Implementation Time per Hook:** 1-2 hours

---

## Next Steps

Want me to implement any of these additional hooks? The most valuable would be:

1. **FileWillBeUploaded** - Critical for security
2. **MessageWillBeDeleted** - Audit compliance
3. **MessageWillBeUpdated** - Message integrity

Let me know which ones you'd like to add!

