# Slack MCP Setup for Claude Desktop

Quick setup guide for getting Slack integration working with Claude Desktop.

## Prerequisites

- [Claude Desktop](https://claude.ai/download) installed
- Go installed (`brew install go`)

## Step 1: Clone the repo

```bash
cd ~
git clone git@github.com:Baggydawg/slack-mcp-server.git
```

## Step 2: Build

```bash
cd slack-mcp-server
go build -o slack-mcp-server ./cmd/slack-mcp-server
```

## Step 3: Get your Slack user token

1. Go to [api.slack.com/apps](https://api.slack.com/apps)
2. Click **Create New App** → **From manifest**
3. Select the **Sophiq** workspace
4. Paste this manifest:

```json
{
  "display_information": {
    "name": "Slack MCP"
  },
  "oauth_config": {
    "scopes": {
      "user": [
        "channels:history",
        "channels:read",
        "groups:history",
        "groups:read",
        "im:history",
        "im:read",
        "mpim:history",
        "mpim:read",
        "users:read",
        "search:read"
      ]
    }
  },
  "settings": {
    "org_deploy_enabled": false,
    "socket_mode_enabled": false,
    "token_rotation_enabled": false
  }
}
```

5. Click **Create**
6. Go to **OAuth & Permissions** → **Install to Workspace**
7. Copy the **User OAuth Token** (starts with `xoxp-`)

## Step 4: Configure Claude Desktop

Edit your Claude Desktop config file:

- **macOS:** `~/Library/Application Support/Claude/claude_desktop_config.json`

Add this inside `"mcpServers"`:

```json
"slack": {
  "command": "/Users/YOURUSERNAME/slack-mcp-server/slack-mcp-server",
  "args": ["--transport", "stdio"],
  "env": {
    "SLACK_MCP_XOXP_TOKEN": "xoxp-your-token-here"
  }
}
```

Replace `YOURUSERNAME` with your macOS username and add your token.

## Step 5: Restart Claude Desktop

Quit Claude Desktop (`Cmd+Q`) and reopen it. Enable the Slack connector in your conversation.

---

Any issues, ask Tobias or ping **#tech**.
