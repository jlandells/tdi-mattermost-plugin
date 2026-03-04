# Implementation Summary

## Overview

The TDI Mattermost Policy Plugin integrates Mattermost with TDI (formerly Direktiv) for policy enforcement and audit. It implements **23 plugin hooks** that call TDI over HTTP for approve/deny decisions and runs fail-secure (deny on error).

---

## Implemented Hooks (23)

### Blocking Hooks (7)

| Hook | Config Key | Description |
|------|------------|-------------|
| MessageWillBePosted | `EnableMessagePolicy` | Approve/deny messages before posting |
| MessageWillBeUpdated | `EnableMessageEditPolicy` | Approve/deny message edits |
| FileWillBeUploaded | `EnableFileUploadPolicy` | Approve/deny file uploads |
| UserWillLogIn | `EnableLoginPolicy` | Approve/deny user login |
| UserHasJoinedChannel | `EnableChannelJoinPolicy` | Check channel joins; can remove user post-join |
| NotificationWillBePushed | `EnablePushNotificationPolicy` | Approve/deny push notifications (9.0+) |
| ConfigurationWillBeSaved | `EnableConfigValidationPolicy` | Validate server config before save (8.0+) |

### Audit Hooks (15)

| Hook | Config Key | Description |
|------|------------|-------------|
| MessageHasBeenPosted | `EnableMessagePostedPolicy` | Report new messages |
| MessageHasBeenUpdated | `EnableMessageUpdatedPolicy` | Report message edits |
| MessageHasBeenDeleted | `EnableMessageDeletePolicy` | Report deletions (audit only; Mattermost cannot block) |
| UserHasLoggedIn | `EnableUserLoggedInPolicy` | Report successful logins |
| ChannelHasBeenCreated | `EnableChannelCreationPolicy` | Auto-classify channels, report creation |
| UserHasJoinedTeam | `EnableTeamJoinPolicy` | Report/restrict team joins |
| UserHasLeftTeam | `EnableUserLeftTeamPolicy` | Report team leaves |
| UserHasLeftChannel | `EnableUserLeftChannelPolicy` | Report channel leaves |
| ReactionHasBeenAdded | `EnableReactionPolicy` | Report/restrict emoji reactions |
| ReactionHasBeenRemoved | `EnableReactionPolicy` | Report reaction removal |
| UserHasBeenCreated | `EnableUserCreatedPolicy` | Report new users |
| UserHasBeenDeactivated | `EnableUserDeactivatedPolicy` | Report user deactivation (9.1+) |
| OnSAMLLogin | `EnableSAMLLoginPolicy` | Report SAML logins (10.7+) |

### Special (1)

| Hook | Config Key | Description |
|------|------------|-------------|
| MessagesWillBeConsumed | `EnableMessagesConsumedPolicy` | Report messages before they reach client (9.3+) |

---

## Key Files

| File | Role |
|------|------|
| `main.go` | Plugin hooks, policy requests to TDI, user attribute extraction |
| `configuration.go` | Config struct, OnConfigurationChange |
| `plugin.json` | Plugin manifest, settings schema |
| `PLUGIN_HOOKS.md` | Full hook list and TDI policy paths |
| `INSTALLATION.md` | Installation and configuration guide |

---

## TDI Policy Paths

The plugin calls TDI at `{TDIURL}/ns/{namespace}/policy/v1/{path}`. See [PLUGIN_HOOKS.md](PLUGIN_HOOKS.md#tdi-policy-paths) for the full list.

Example paths:
- `message/check` — MessageWillBePosted
- `channel/join` — UserHasJoinedChannel
- `message/edit` — MessageWillBeUpdated
- `message/delete` — MessageHasBeenDeleted
- `file/upload` — FileWillBeUploaded
- `user/login` — UserWillLogIn
- `channel/create` — ChannelHasBeenCreated
- …plus 16 more for audit and other policies

---

## User Attributes

The plugin sends user attributes to TDI for policy evaluation:

- **Built-in**: `username`, `email`, `roles`, `first_name`, `last_name`, `nickname`, `position`
- **LDAP** (when `AuthService == "ldap"`): via `GetLDAPUserAttributes`
- **Custom profile** (Enterprise 10.10+): via Property API (`SearchPropertyValues`, `SearchPropertyFields`)
- **UserAttributeMapping**: Maps policy keys to Mattermost/LDAP fields (e.g. `{"clearance": "employeeClearance"}`)

---

## Architecture

```
User Action → Plugin Hook → HTTP POST → TDI Gateway → Workflow → Allow/Deny
```

- **Fail-secure**: On TDI error or timeout, the plugin denies the action
- **Configurable**: Each policy can be enabled/disabled via plugin settings
- **Admin exemption**: Optional bypass for system admins via `ExemptSystemAdmins`

---

## Stats

| Metric | Value |
|--------|-------|
| Total hooks | 23 |
| Blocking | 7 |
| Audit | 15 |
| Special | 1 |
| Config options | 20 policy toggles + 5 advanced |
| Min Mattermost | 9.0.0 |

---

## Documentation

- [PLUGIN_HOOKS.md](PLUGIN_HOOKS.md) — Hooks and TDI paths
- [INSTALLATION.md](INSTALLATION.md) — Setup and configuration
- [BUILD_NOTES.md](BUILD_NOTES.md) — Build requirements

---

**Status:** ✅ Complete  
**Ready for:** Deployment and testing
