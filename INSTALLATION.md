# Installation Guide

This guide walks you through installing and configuring the Mattermost Policy Plugin with Direktiv.

## Prerequisites

Before starting, ensure you have:

- **Mattermost Server 9.0+** installed and running
- **Direktiv instance** accessible from your Mattermost server
- **System Administrator** access to Mattermost
- **Go 1.21+** installed (for building the plugin)
- **Direct network access** from Mattermost server to Direktiv

## Step 1: Deploy Direktiv Components

### 1.1 Create Namespace

Create a namespace in Direktiv for your Mattermost policies:

```bash
# Using Direktiv CLI or UI
direktiv namespace create mattermost-policies
```

### 1.2 Deploy Workflows

Upload the workflow files to your Direktiv namespace:

```bash
# Navigate to the workflows directory
cd workflows/

# Upload via Direktiv UI or CLI
# File: message-policy.yaml → /workflows/message-policy.yaml
# File: channel-join-policy.yaml → /workflows/channel-join-policy.yaml
```

**Via Direktiv UI:**
1. Navigate to your namespace
2. Go to Workflows section
3. Create new workflow
4. Copy contents of each YAML file
5. Save

### 1.3 Deploy Gateways

Upload the gateway definitions:

```bash
# Navigate to the gateways directory
cd gateways/

# Upload via Direktiv UI or CLI
# File: message-policy.yaml → /gateways/message-policy.yaml
# File: channel-join-policy.yaml → /gateways/channel-join-policy.yaml
```

**Via Direktiv UI:**
1. Navigate to your namespace
2. Go to Gateway/Endpoints section
3. Create new endpoint
4. Copy contents of each YAML file
5. Save and activate

### 1.4 Test Direktiv Endpoints

Verify the endpoints are accessible:

```bash
# Test message policy endpoint
curl -X POST https://your-direktiv-instance.com/ns/mattermost-policies/policy/v1/message/check \
  -H "Content-Type: application/json" \
  -d @data/sample-message-request.json

# Expected response:
# {
#   "status": "success",
#   "action": "continue",
#   "result": {}
# }

# Test channel join policy endpoint
curl -X POST https://your-direktiv-instance.com/ns/mattermost-policies/policy/v1/channel/join \
  -H "Content-Type: application/json" \
  -d @data/sample-channel-join-request.json
```

## Step 2: Build Mattermost Plugin

### 2.1 Install Dependencies

```bash
cd plugin/
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

# This creates: dist/com.archtis.mattermost-policy-plugin-1.0.0.tar.gz
```

## Step 3: Install Plugin in Mattermost

### 3.1 Upload Plugin

1. Log in to Mattermost as **System Administrator**
2. Navigate to **System Console** → **Plugins** → **Plugin Management**
3. Click **Upload Plugin**
4. Select the bundle file: `com.archtis.mattermost-policy-plugin-1.0.0.tar.gz`
5. Click **Upload**

### 3.2 Enable Plugin

1. Find **Mattermost Policy Plugin** in the plugin list
2. Click **Enable**
3. The plugin status should show as **Running**

## Step 4: Configure Plugin

### 4.1 Basic Configuration

1. Navigate to **System Console** → **Plugins** → **Mattermost Policy Plugin**
2. Configure the following settings:

| Setting | Value | Description |
|---------|-------|-------------|
| **Direktiv Base URL** | `https://your-direktiv-instance.com` | Your Direktiv instance URL (no trailing slash) |
| **Direktiv Namespace** | `mattermost-policies` | The namespace containing your workflows |
| **Enable Message Policy Checks** | `true` | Enable message policy enforcement |
| **Enable Channel Join Policy Checks** | `true` | Enable channel join policy enforcement |
| **Policy Request Timeout** | `5` | Seconds to wait for policy response |

3. Click **Save**

### 4.2 Optional Configuration

**Direktiv API Key** (if your Direktiv instance requires authentication):
```
your-api-key-here
```

**Enable Debug Logging** (for troubleshooting):
```
true
```

**Exempt System Admins** (allow admins to bypass policies):
```
false  # Set to true if you want admins to bypass all policies
```

**User Attribute Mapping** (map Mattermost fields to policy attributes):
```json
{
  "clearance": "CustomAttribute1",
  "department": "CustomAttribute2",
  "nationality": "CustomAttribute3"
}
```

## Step 5: Verify Installation

### 5.1 Check Plugin Logs

View Mattermost logs to confirm plugin activation:

```bash
# Mattermost log location varies by installation
tail -f /opt/mattermost/logs/mattermost.log | grep "mattermost-policy-plugin"

# Expected output:
# [INFO] Mattermost Policy Plugin activated direktiv_url=https://... namespace=mattermost-policies
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

**Verify Direktiv connectivity:**
```bash
# From Mattermost server
curl -v https://your-direktiv-instance.com/ns/mattermost-policies/policy/v1/message/check
```

**Common issues:**
- Direktiv URL incorrect or unreachable
- Network firewall blocking requests
- Direktiv namespace doesn't exist
- Workflows not deployed

### Policies Not Evaluating Correctly

**Enable debug logging:**
1. System Console → Plugins → Mattermost Policy Plugin
2. Set **Enable Debug Logging** to `true`
3. Review logs for policy requests and responses

**Check workflow logic:**
1. Test workflows directly in Direktiv UI
2. Verify jq expressions in workflow conditions
3. Check user attribute values

### Performance Issues

**Increase timeout:**
1. Increase **Policy Request Timeout** to 10 seconds
2. Monitor Direktiv workflow execution times

**Optimize workflows:**
1. Remove unnecessary logging
2. Simplify jq expressions
3. Consider caching common decisions

## Security Best Practices

1. **Always use HTTPS** for Direktiv communication
2. **Enable TLS certificate verification** (don't skip in production)
3. **Use API keys** for Direktiv authentication
4. **Rotate API keys** regularly
5. **Audit policy decisions** via Direktiv logging
6. **Test policies thoroughly** before production deployment
7. **Have a fallback plan** if Direktiv becomes unavailable

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

To remove Direktiv components:

1. Delete workflows from Direktiv namespace
2. Delete gateway endpoints
3. Optionally delete the namespace

## Support

For issues or questions:

- Check plugin logs for errors
- Test Direktiv workflows independently
- Review Direktiv documentation at https://docs.direktiv.io/
- Contact your security team for policy-related questions

## Next Steps

After installation:

1. **Customize workflows** - Modify policies to match your organization's requirements
2. **Test extensively** - Verify all policy scenarios work as expected
3. **Train users** - Educate users on channel classification and access restrictions
4. **Monitor logs** - Set up monitoring for policy decisions and denials
5. **Iterate** - Refine policies based on usage patterns and feedback

