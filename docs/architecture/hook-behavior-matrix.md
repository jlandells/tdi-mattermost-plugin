# Hook Behavior Matrix

This matrix tracks the intended production behavior for each Mattermost hook.
Validate every row against the Mattermost server plugin SDK reference before
shipping a production release.

| Hook | Config Key | Timing | Intended Behavior | TDI Path | Production Status |
| --- | --- | --- | --- | --- | --- |
| `MessageWillBePosted` | `EnableMessagePolicy` | Before commit | Block or allow | `message/check` | Needs tests |
| `MessageWillBeUpdated` | `EnableMessageEditPolicy` | Before commit | Block or allow | `message/edit` | Needs tests |
| `FileWillBeUploaded` | `EnableFileUploadPolicy` | Before commit | Block or allow | `file/upload` | Bounded spool/hash implemented; needs integration test |
| `UserWillLogIn` | `EnableLoginPolicy` | Before login completes | Block or allow | `user/login` | Needs SDK verification |
| `UserHasJoinedChannel` | `EnableChannelJoinPolicy` | After join | Remove user if denied | `channel/join` | Remediation only |
| `UserHasJoinedTeam` | `EnableTeamJoinPolicy` | After join | Remove user if denied | `team/join` | Remediation only |
| `ReactionHasBeenAdded` | `EnableReactionPolicy` | After reaction | Remove reaction if denied | `reaction/add` | Remediation only |
| `MessageHasBeenDeleted` | `EnableMessageDeletePolicy` | After delete | Audit only | `message/delete` | Audit only |
| `MessageHasBeenPosted` | `EnableMessagePostedPolicy` | After post | Audit only | `message/posted` | Audit only |
| `MessageHasBeenUpdated` | `EnableMessageUpdatedPolicy` | After update | Audit only | `message/updated` | Audit only |
| `UserHasLoggedIn` | `EnableUserLoggedInPolicy` | After login | Audit only | `user/logged_in` | Audit only |
| `UserHasBeenCreated` | `EnableUserCreatedPolicy` | After create | Audit/onboarding | `user/created` | Audit only |
| `UserHasBeenDeactivated` | `EnableUserDeactivatedPolicy` | After deactivate | Audit only | `user/deactivated` | Audit only |
| `UserHasLeftTeam` | `EnableUserLeftTeamPolicy` | After leave | Audit only | `team/leave` | Audit only |
| `UserHasLeftChannel` | `EnableUserLeftChannelPolicy` | After leave | Audit only | `channel/leave` | Audit only |
| `ChannelHasBeenCreated` | `EnableChannelCreationPolicy` | After create | Audit/classification | `channel/create` | Needs product decision |
| `NotificationWillBePushed` | `EnablePushNotificationPolicy` | Before push | Block or allow | `notification/push` | Needs SDK verification |
| `ConfigurationWillBeSaved` | `EnableConfigValidationPolicy` | Before save | Block or allow | `config/validate` | Needs tests |
| `OnSAMLLogin` | `EnableSAMLLoginPolicy` | Login flow | Audit only | `saml/login` | Needs version verification |
| `MessagesWillBeConsumed` | `EnableMessagesConsumedPolicy` | Before client receive | Audit/filter candidate | `messages/consumed` | Needs design decision |
