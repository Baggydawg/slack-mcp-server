# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Slack MCP Server is a Model Context Protocol (MCP) server that bridges Claude and other AI models with Slack workspaces. It provides tools to read messages, search conversations, access user/channel metadata, and optionally post messages. Supports multiple transports (stdio, SSE, HTTP) and authentication modes (OAuth tokens, bot tokens, stealth browser tokens).

## Build & Development Commands

```bash
make build                # Build binary (includes clean, tidy, format)
make test                 # Run unit tests (tests matching *Unit*)
make test-integration     # Run integration tests (requires SLACK_MCP_OPENAI_API env var)
make format               # Format code with go fmt
make tidy                 # Tidy go modules
make deps                 # Download dependencies
make build-all-platforms  # Cross-compile for darwin/linux/windows x amd64/arm64
make build-dxt            # Build Anthropic DXT extension
make npm-publish          # Publish platform-specific npm packages
make release TAG=v1.0.0   # Create and push release tag
```

Run a single test:
```bash
go test -v -run "TestNameHere" ./pkg/...
```

## Architecture

### Entry Point & Transport Modes

Main entry point is `cmd/slack-mcp-server/main.go`. The server supports three transport modes:
- **stdio** (default): Communicates via stdin/stdout for MCP inspector
- **sse**: Server-Sent Events HTTP server for persistent connections
- **http**: Standard HTTP server

### Core Package Structure

```
pkg/
├── handler/        # MCP tool implementations
├── provider/       # Slack API abstraction layer
│   └── edge/       # Undocumented Slack Edge API client
├── server/         # MCP server setup with mark3labs/mcp-go
├── transport/      # HTTP transport with uTLS fingerprinting
├── text/           # Text processing, URL parsing
├── limiter/        # Rate limiting
└── version/        # Build version info
```

### Key Architectural Patterns

**Dual API Clients**: The `ApiProvider` in `pkg/provider/` combines:
- Standard `slack-go/slack` client for official Slack API
- Custom `edge.Client` for undocumented Slack Edge APIs (stealth mode)

**Cache-First Startup**: The server blocks until user/channel caches are warmed. This is critical - tools resolve channels/users by name (e.g., `#general`, `@user`) via these caches. See `RefreshUsers()` and `RefreshChannels()` in provider.

**Authentication Modes**:
- `xoxp-*`: User OAuth tokens (full access)
- `xoxb-*`: Bot tokens (limited to invited channels, no search)
- `xoxc-*` + `xoxd-*`: Browser session tokens (stealth mode, no permissions needed)

**TLS Fingerprinting**: `pkg/transport/` uses uTLS library to match specific browser TLS signatures for Enterprise Slack environments.

### MCP Tools (in `pkg/handler/`)

1. `conversations_history` - Fetch channel/DM messages with pagination
2. `conversations_replies` - Fetch thread replies
3. `conversations_add_message` - Post messages (disabled by default)
4. `conversations_search_messages` - Search across workspace
5. `channels_list` - Get available channels

### MCP Resources

- `slack://<workspace>/channels` - CSV directory of all channels
- `slack://<workspace>/users` - CSV directory of all users

## Testing

Unit tests use the `*Unit*` naming convention, integration tests use `*Integration*`. Integration tests require:
- `SLACK_MCP_OPENAI_API` environment variable
- Tests use ngrok tunneling for external connectivity

## Key Dependencies

- `github.com/mark3labs/mcp-go` - MCP protocol implementation
- `github.com/slack-go/slack` - Official Slack Go SDK
- `github.com/rusq/slack` - Enhanced Slack client with Edge API
- `go.uber.org/zap` - Structured logging
- `github.com/refraction-networking/utls` - TLS fingerprinting

## Important Environment Variables

- `SLACK_MCP_ADD_MESSAGE_TOOL` - Enable message posting (disabled by default for safety)
- `SLACK_MCP_LOG_LEVEL` - Set to `debug` for verbose logging
- `SLACK_MCP_CUSTOM_TLS` - Enable custom TLS handshakes for Enterprise environments
