# Hook Behavior Matrix

This matrix tracks the intended behavior for each Mattermost hook. Hook timing
determines whether a policy can block the original action, remediate after the
action, or only audit.

| Hook | Config Key | Timing | Intended Behavior | TDI Path | Production Status |
| --- | --- | --- | --- | --- | --- |
| `MessageWillBePosted` | `EnableMessagePolicy` | Before commit | Block or allow | `policy/v1/message/check` | Unit covered; needs integration coverage |
| `MessageWillBeUpdated` | `EnableMessageEditPolicy` | Before commit | Block or allow | `policy/v1/message/edit` | Needs integration coverage |
| `FileWillBeUploaded` | `EnableFileUploadPolicy` | Before commit | Block or allow | `policy/v1/file/upload` | Bounded spool/hash implemented; unit covered |
| `UserWillLogIn` | `EnableLoginPolicy` | Before login completes | Block or allow | `policy/v1/user/login` | Needs integration coverage |
| `NotificationWillBePushed` | `EnablePushNotificationPolicy` | Before push | Block or allow | `policy/v1/notification/push` | Needs integration coverage |
| `ConfigurationWillBeSaved` | `EnableConfigValidationPolicy` | Before save | Block or allow | `policy/v1/config/validate` | Config validation unit covered |
| `UserHasJoinedChannel` | `EnableChannelJoinPolicy` | After join | Remove user if denied | `policy/v1/channel/join` | Remediation only |
| `UserHasJoinedTeam` | `EnableTeamJoinPolicy` | After join | Remove user if denied | `policy/v1/team/join` | Remediation only |
| `ReactionHasBeenAdded` | `EnableReactionPolicy` | After reaction | Remove reaction if denied | `policy/v1/reaction/add` | Remediation only |
| `MessageHasBeenDeleted` | `EnableMessageDeletePolicy` | After delete | Audit only | `policy/v1/message/delete` | Audit only |
| `MessageHasBeenPosted` | `EnableMessagePostedPolicy` | After post | Audit only | `policy/v1/message/posted` | Audit only |
| `MessageHasBeenUpdated` | `EnableMessageUpdatedPolicy` | After update | Audit only | `policy/v1/message/updated` | Audit only |
| `UserHasLoggedIn` | `EnableUserLoggedInPolicy` | After login | Audit only | `policy/v1/user/logged_in` | Audit only |
| `UserHasBeenCreated` | `EnableUserCreatedPolicy` | After create | Audit/onboarding | `policy/v1/user/created` | Audit only |
| `UserHasBeenDeactivated` | `EnableUserDeactivatedPolicy` | After deactivate | Audit only | `policy/v1/user/deactivated` | Audit only |
| `UserHasLeftTeam` | `EnableUserLeftTeamPolicy` | After leave | Audit only | `policy/v1/team/leave` | Audit only |
| `UserHasLeftChannel` | `EnableUserLeftChannelPolicy` | After leave | Audit only | `policy/v1/channel/leave` | Audit only |
| `ReactionHasBeenRemoved` | `EnableReactionPolicy` | After reaction removal | Audit only | `policy/v1/reaction/remove` | Audit only |
| `ChannelHasBeenCreated` | `EnableChannelCreationPolicy` | After create | Audit/classification notice | `policy/v1/channel/create` | Audit/remediation only |
| `OnSAMLLogin` | `EnableSAMLLoginPolicy` | Login flow | Audit only | `policy/v1/saml/login` | Requires Mattermost version support |
| `MessagesWillBeConsumed` | `EnableMessagesConsumedPolicy` | Before client receive | Audit only | `policy/v1/messages/consumed` | Filtering not implemented |
