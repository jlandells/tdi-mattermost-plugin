# Plugin Hooks

This document lists the Mattermost hooks used by the plugin and the TDI policy
paths they call. The behavior depends on Mattermost hook timing.

## Blocking Hooks

These hooks run before Mattermost completes the action and can reject the user
operation.

| Hook | Config Key | TDI Path |
| --- | --- | --- |
| `MessageWillBePosted` | `EnableMessagePolicy` | `policy/v1/message/check` |
| `MessageWillBeUpdated` | `EnableMessageEditPolicy` | `policy/v1/message/edit` |
| `FileWillBeUploaded` | `EnableFileUploadPolicy` | `policy/v1/file/upload` |
| `UserWillLogIn` | `EnableLoginPolicy` | `policy/v1/user/login` |
| `NotificationWillBePushed` | `EnablePushNotificationPolicy` | `policy/v1/notification/push` |
| `ConfigurationWillBeSaved` | `EnableConfigValidationPolicy` | `policy/v1/config/validate` |

## Remediation Hooks

These hooks run after Mattermost has accepted the action. The plugin may take a
follow-up action, such as removing a user from a channel or team.

| Hook | Config Key | TDI Path |
| --- | --- | --- |
| `UserHasJoinedChannel` | `EnableChannelJoinPolicy` | `policy/v1/channel/join` |
| `UserHasJoinedTeam` | `EnableTeamJoinPolicy` | `policy/v1/team/join` |
| `ReactionHasBeenAdded` | `EnableReactionPolicy` | `policy/v1/reaction/add` |

## Audit Hooks

These hooks report completed events to TDI and do not block the original
Mattermost action.

| Hook | Config Key | TDI Path |
| --- | --- | --- |
| `MessageHasBeenPosted` | `EnableMessagePostedPolicy` | `policy/v1/message/posted` |
| `MessageHasBeenUpdated` | `EnableMessageUpdatedPolicy` | `policy/v1/message/updated` |
| `MessageHasBeenDeleted` | `EnableMessageDeletePolicy` | `policy/v1/message/delete` |
| `UserHasLoggedIn` | `EnableUserLoggedInPolicy` | `policy/v1/user/logged_in` |
| `ChannelHasBeenCreated` | `EnableChannelCreationPolicy` | `policy/v1/channel/create` |
| `UserHasLeftTeam` | `EnableUserLeftTeamPolicy` | `policy/v1/team/leave` |
| `UserHasLeftChannel` | `EnableUserLeftChannelPolicy` | `policy/v1/channel/leave` |
| `ReactionHasBeenRemoved` | `EnableReactionPolicy` | `policy/v1/reaction/remove` |
| `UserHasBeenCreated` | `EnableUserCreatedPolicy` | `policy/v1/user/created` |
| `MessagesWillBeConsumed` | `EnableMessagesConsumedPolicy` | `policy/v1/messages/consumed` |
| `UserHasBeenDeactivated` | `EnableUserDeactivatedPolicy` | `policy/v1/user/deactivated` |
| `OnSAMLLogin` | `EnableSAMLLoginPolicy` | `policy/v1/saml/login` |

## Not Implemented

| Hook | Reason |
| --- | --- |
| `FileWillBeDownloaded` | Requires a Mattermost server/plugin SDK version that exposes this hook. |
