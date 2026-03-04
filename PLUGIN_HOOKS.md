# Mattermost Plugin Hooks Reference

This document describes the Mattermost plugin hooks used by the TDI Mattermost Policy Plugin for policy enforcement and audit.

---

## Implemented Hooks (23)

All policy-related hooks below are **implemented** and configurable via the plugin settings.

### Blocking Hooks (approve/deny before action)

| Hook | Config Key | Description |
|------|------------|-------------|
| **MessageWillBePosted** | `EnableMessagePolicy` | Approve/deny messages before posting |
| **MessageWillBeUpdated** | `EnableMessageEditPolicy` | Approve/deny message edits |
| **FileWillBeUploaded** | `EnableFileUploadPolicy` | Approve/deny file uploads |
| **UserWillLogIn** | `EnableLoginPolicy` | Approve/deny user login |
| **UserHasJoinedChannel** | `EnableChannelJoinPolicy` | Check when user joins channel; can remove user post-join |
| **NotificationWillBePushed** | `EnablePushNotificationPolicy` | Approve/deny push notifications (server 9.0+) |
| **ConfigurationWillBeSaved** | `EnableConfigValidationPolicy` | Validate server config via TDI before saving (server 8.0+) |

### Audit Hooks (report to TDI after action)

| Hook | Config Key | Description |
|------|------------|-------------|
| **MessageHasBeenPosted** | `EnableMessagePostedPolicy` | Report new messages (audit) |
| **MessageHasBeenUpdated** | `EnableMessageUpdatedPolicy` | Report message edits (audit) |
| **MessageHasBeenDeleted** | `EnableMessageDeletePolicy` | Report message deletions (audit) |
| **UserHasLoggedIn** | `EnableUserLoggedInPolicy` | Report successful logins (audit) |
| **ChannelHasBeenCreated** | `EnableChannelCreationPolicy` | Auto-classify channels, report creation |
| **UserHasJoinedTeam** | `EnableTeamJoinPolicy` | Report user joining team |
| **UserHasLeftTeam** | `EnableUserLeftTeamPolicy` | Report user leaving team |
| **UserHasLeftChannel** | `EnableUserLeftChannelPolicy` | Report user leaving channel |
| **ReactionHasBeenAdded** | `EnableReactionPolicy` | Report emoji reaction added |
| **ReactionHasBeenRemoved** | `EnableReactionPolicy` | Report emoji reaction removed |
| **UserHasBeenCreated** | `EnableUserCreatedPolicy` | Report new user creation |
| **UserHasBeenDeactivated** | `EnableUserDeactivatedPolicy` | Report user deactivation (server 9.1+) |
| **OnSAMLLogin** | `EnableSAMLLoginPolicy` | Report SAML logins (audit; server 10.7+) |

### Special Hooks

| Hook | Config Key | Description |
|------|------------|-------------|
| **MessagesWillBeConsumed** | `EnableMessagesConsumedPolicy` | Report messages before they reach the client (audit; server 9.3+) |

---

## Not Implemented

| Hook | Min Server | Note |
|------|------------|------|
| **FileWillBeDownloaded** | 11.5 | Requires plugin SDK with this hook; not in current dependency |

---

## Summary

| Category | Count |
|----------|-------|
| **Implemented** | 23 hooks |
| **Blocking** | 7 |
| **Audit** | 15 |
| **Special** | 1 |

---

## TDI Policy Paths

The plugin calls TDI at these paths (when the corresponding config is enabled):

| Path | Hook |
|------|------|
| `policy/v1/message/check` | MessageWillBePosted |
| `policy/v1/channel/join` | UserHasJoinedChannel |
| `policy/v1/message/edit` | MessageWillBeUpdated |
| `policy/v1/message/delete` | MessageHasBeenDeleted |
| `policy/v1/file/upload` | FileWillBeUploaded |
| `policy/v1/user/login` | UserWillLogIn |
| `policy/v1/channel/create` | ChannelHasBeenCreated |
| `policy/v1/channel/leave` | UserHasLeftChannel |
| `policy/v1/team/join` | UserHasJoinedTeam |
| `policy/v1/team/leave` | UserHasLeftTeam |
| `policy/v1/reaction/add` | ReactionHasBeenAdded |
| `policy/v1/reaction/remove` | ReactionHasBeenRemoved |
| `policy/v1/user/created` | UserHasBeenCreated |
| `policy/v1/message/posted` | MessageHasBeenPosted |
| `policy/v1/message/updated` | MessageHasBeenUpdated |
| `policy/v1/user/logged_in` | UserHasLoggedIn |
| `policy/v1/messages/consumed` | MessagesWillBeConsumed |
| `policy/v1/user/deactivated` | UserHasBeenDeactivated |
| `policy/v1/notification/push` | NotificationWillBePushed |
| `policy/v1/config/validate` | ConfigurationWillBeSaved |
| `policy/v1/saml/login` | OnSAMLLogin |

---

## Adding a New Hook

1. **Add the hook** to `main.go` with the correct Mattermost signature.
2. **Add a config option** in `configuration.go` and `plugin.json`.
3. **Add TDI gateway and workflow** in `tdi-mattermost-workflows` (if using that layout).
4. **Guard with config** so the hook returns early when its feature is disabled.
