# Installation Guide

This guide walks you through installing and configuring the Mattermost Policy Plugin with TDI.

## Prerequisites

Before starting, ensure you have:

- **Mattermost Server 9.0+** installed and running
- **TDI instance** accessible from your Mattermost server
- **System Administrator** access to Mattermost
- **Go 1.21+** installed (for building the plugin)
- **Direct network access** from Mattermost server to TDI

## Step 1: Deploy TDI Components

### 1.1 Create Namespace

Create a namespace in TDI for your Mattermost policies:

```bash
# Using TDI CLI or UI
tdi namespace create mattermost-policies
```

### 1.2 Deploy Workflows and Gateways

The plugin calls TDI at policy paths under `policy/v1/`. For each policy you enable, deploy a corresponding gateway and workflow.

**Minimum for default config** (Message + Channel Join policy):
- `policy/v1/message/check` — MessageWillBePosted
- `policy/v1/channel/join` — UserHasJoinedChannel

**Full list of policy paths** — see [PLUGIN_HOOKS.md](PLUGIN_HOOKS.md#tdi-policy-paths) for all paths and which config enables them.

If using the companion `tdi-mattermost-workflows` package, workflows and gateways are in its `workflows/` and `gateways/` directories. Upload via TDI UI:
1. Navigate to your namespace
2. **Workflows:** Create workflow for each policy (e.g. `message-policy.yaml`, `channel-join-policy.yaml`)
3. **Gateways:** Create endpoints that invoke those workflows at the matching paths

### 1.3 Test TDI Endpoints

Verify the endpoints you use are accessible:

```bash
# Test message policy endpoint
curl -X POST https://your-tdi-instance.com/ns/mattermost-policies/policy/v1/message/check \
  -H "Content-Type: application/json" \
  -d '{"user_id":"test","username":"test","channel_name":"general","message":"hello","action":"message"}'

# Expected response:
# {"status":"success","action":"continue","result":{}}

# Test channel join policy endpoint
curl -X POST https://your-tdi-instance.com/ns/mattermost-policies/policy/v1/channel/join \
  -H "Content-Type: application/json" \
  -d '{"user_id":"test","username":"test","channel_name":"general","action":"channel_join"}'
```

## Step 2: Build Mattermost Plugin

### 2.1 Install Dependencies

```bash
cd tdi-mattermost-plugins/
go mod download
go mod tidy
```

### 2.2 Build Plugin

```bash
# Build for all platforms
make build

# Or build for specific platform
make build-linux    # Linux AMD64/ARM64
make build-darwin   # macOS AMD64/ARM64
make build-windows  # Windows AMD64
```

### 2.3 Create Plugin Bundle

```bash
make bundle

# This creates: dist/com.archtis.mattermost-policy-plugin-1.0.2.tar.gz
```

## Step 3: Install Plugin in Mattermost

### 3.1 Upload Plugin

1. Log in to Mattermost as **System Administrator**
2. Navigate to **System Console** → **Plugins** → **Plugin Management**
3. Click **Upload Plugin**
4. Select the bundle file: `com.archtis.mattermost-policy-plugin-1.0.2.tar.gz`
5. Click **Upload**

### 3.2 Enable Plugin

1. Find **Mattermost Policy Plugin** in the plugin list
2. Click **Enable**
3. The plugin status should show as **Running**

## Step 4: Configure Plugin

### 4.1 Required Settings

1. Navigate to **System Console** → **Plugins** → **Mattermost Policy Plugin**
2. Set the required values:

| Setting | Value | Description |
|---------|-------|-------------|
| **TDI Base URL** | `https://your-tdi-instance.com` | Your TDI instance URL (no trailing slash) |
| **TDI Namespace** | `mattermost-policies` | The namespace containing your workflows |
| **Policy Request Timeout** | `5` | Seconds to wait for policy response |

3. Click **Save**

### 4.2 Policy Toggles

Enable only the policies you have workflows for. Each enabled policy calls TDI at the corresponding path (see [PLUGIN_HOOKS.md](PLUGIN_HOOKS.md)).

| Policy | Config Key | Default | Description |
|--------|------------|---------|-------------|
| **Message (block)** | Enable Message Policy Checks | `true` | Block posts before they are saved |
| **Channel Join** | Enable Channel Join Policy Checks | `true` | Check when users join channels; can remove them |
| **Message Edit** | Enable Message Edit Policy Checks | `false` | Block message edits |
| **Message Delete (audit)** | Enable Message Delete Policy (audit) | `false` | Report deletions to TDI |
| **File Upload** | Enable File Upload Policy Checks | `false` | Block file uploads |
| **Login** | Enable Login Policy Checks | `false` | Block user login |
| **Channel Creation** | Enable Channel Creation Policy | `false` | Auto-classify channels, report creation |
| **Reactions** | Enable Reaction Policy Checks | `false` | Report/restrict emoji reactions |
| **User Created (audit)** | Enable User Created Policy (audit) | `false` | Report new users |
| **Team Join** | Enable Team Join Policy Checks | `false` | Check when users join teams |
| **User Left Team (audit)** | Enable User Left Team Policy (audit) | `false` | Report users leaving teams |
| **User Left Channel (audit)** | Enable User Left Channel Policy (audit) | `false` | Report users leaving channels |
| **Message Posted (audit)** | Enable Message Posted Policy (audit) | `false` | Report new messages |
| **Message Updated (audit)** | Enable Message Updated Policy (audit) | `false` | Report message edits |
| **User Logged In (audit)** | Enable User Logged In Policy (audit) | `false` | Report successful logins |
| **Messages Consumed** | Enable Messages Consumed Policy | `false` | Report messages before they reach client (9.3+) |
| **User Deactivated (audit)** | Enable User Deactivated Policy (audit) | `false` | Report user deactivation (9.1+) |
| **Push Notification** | Enable Push Notification Policy | `false` | Block/modify push notifications |
| **Config Validation** | Enable Config Validation Policy | `false` | Validate server config before save |
| **SAML Login (audit)** | Enable SAML Login Policy (audit) | `false` | Report SAML logins (10.7+) |

### 4.3 Advanced Settings

| Setting | Description |
|---------|-------------|
| **TDI API Key** | API key for TDI authentication (optional) |
| **Enable Debug Logging** | Log policy requests/responses (for troubleshooting) |
| **Exempt System Admins** | Bypass all policy checks for system admins |
| **User Attribute Mapping** | JSON mapping of policy attribute names to Mattermost/LDAP fields |

Example User Attribute Mapping:
```json
{
  "clearance": "employeeClearance",
  "department": "department",
  "nationality": "country"
}
```

## Step 5: Verify Installation

### 5.1 Check Plugin Logs

View Mattermost logs to confirm plugin activation:

```bash
# Mattermost log location varies by installation
tail -f /opt/mattermost/logs/mattermost.log | grep "mattermost-policy-plugin"

# Expected output:
# [INFO] Mattermost Policy Plugin activated tdi_url=https://... namespace=mattermost-policies
```

### 5.2 Test Message Policy

1. Create a test channel named `secret-test-channel`
2. Set channel header to: `CLEARANCE_REQUIRED=SECRET`
3. As a regular user (without SECRET clearance attribute), try to post a message
4. You should receive a denial message

### 5.3 Test Channel Join Policy

1. Create a test channel named `ts-classified`
2. As a user without TOP SECRET clearance, try to join the channel
3. You should be automatically removed and receive a DM explaining why

## Step 6: Set Up User Attributes

For the policies to work properly, users need attributes like clearance and department.

### Option 1: SAML Attributes

If using SAML authentication, map IdP attributes to Mattermost:

1. Navigate to **System Console** → **Authentication** → **SAML 2.0**
2. Configure attribute mappings:
   - `clearance` → SAML attribute `securityClearance`
   - `department` → SAML attribute `department`
   - `nationality` → SAML attribute `country`

### Option 2: OAuth Attributes

If using OAuth, ensure claims are mapped to user properties.

### Option 3: Manual Attributes via API

Set user attributes via Mattermost API:

```bash
# Set user custom attributes
curl -X PUT https://mattermost.example.com/api/v4/users/{user_id}/patch \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{
    "props": {
      "CustomAttribute1": "SECRET",
      "CustomAttribute2": "engineering",
      "CustomAttribute3": "USA"
    }
  }'
```

## Troubleshooting

### Plugin Won't Activate

**Check Go version:**
```bash
go version  # Should be 1.21 or higher
```

**Check Mattermost logs:**
```bash
tail -f /opt/mattermost/logs/mattermost.log
```

**Common issues:**
- Plugin binary not compatible with Mattermost version
- Missing dependencies in go.mod
- Incorrect file permissions

### Policy Checks Always Deny

**Verify TDI connectivity:**
```bash
# From Mattermost server
curl -v https://your-tdi-instance.com/ns/mattermost-policies/policy/v1/message/check
```

**Common issues:**
- TDI URL incorrect or unreachable
- Network firewall blocking requests
- TDI namespace doesn't exist
- Workflows not deployed

### Policies Not Evaluating Correctly

**Enable debug logging:**
1. System Console → Plugins → Mattermost Policy Plugin
2. Set **Enable Debug Logging** to `true`
3. Review logs for policy requests and responses

**Check workflow logic:**
1. Test workflows directly in TDI UI
2. Verify jq expressions in workflow conditions
3. Check user attribute values

### Performance Issues

**Increase timeout:**
1. Increase **Policy Request Timeout** to 10 seconds
2. Monitor TDI workflow execution times

**Optimize workflows:**
1. Remove unnecessary logging
2. Simplify jq expressions
3. Consider caching common decisions

## Security Best Practices

1. **Always use HTTPS** for TDI communication
2. **Enable TLS certificate verification** (don't skip in production)
3. **Use API keys** for TDI authentication
4. **Rotate API keys** regularly
5. **Audit policy decisions** via TDI logging
6. **Test policies thoroughly** before production deployment
7. **Have a fallback plan** if TDI becomes unavailable

## Updating the Plugin

To update the plugin:

1. Build new version with updated code
2. Create new bundle with incremented version
3. Upload new bundle via System Console
4. The old version will be replaced automatically
5. Plugin will restart with new configuration

## Uninstallation

To remove the plugin:

1. Navigate to **System Console** → **Plugins** → **Plugin Management**
2. Find **Mattermost Policy Plugin**
3. Click **Disable**
4. Click **Remove**
5. Confirm removal

To remove TDI components:

1. Delete workflows from TDI namespace
2. Delete gateway endpoints
3. Optionally delete the namespace

## Support

For issues or questions:

- Check plugin logs for errors
- Test TDI workflows independently
- Review TDI documentation at https://docs.tdi.io/
- Contact your security team for policy-related questions

## Next Steps

After installation:

1. **Review policy hooks** — See [PLUGIN_HOOKS.md](PLUGIN_HOOKS.md) for all 23 hooks and TDI policy paths
2. **Customize workflows** — Deploy TDI gateways and workflows for each enabled policy
3. **Test extensively** — Verify all policy scenarios work as expected
4. **Train users** — Educate users on channel classification and access restrictions
5. **Monitor logs** — Set up monitoring for policy decisions and denials

