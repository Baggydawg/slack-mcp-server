package handler

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/korotovsky/slack-mcp-server/pkg/provider"
	"github.com/mark3labs/mcp-go/mcp"
	"go.uber.org/zap"
)

type TeamContextHandler struct {
	apiProvider *provider.ApiProvider
	logger      *zap.Logger
}

func NewTeamContextHandler(apiProvider *provider.ApiProvider, logger *zap.Logger) *TeamContextHandler {
	return &TeamContextHandler{
		apiProvider: apiProvider,
		logger:      logger,
	}
}

// resolveChannelInput resolves channel reference to (id, displayName, found)
// Supported formats:
//   - "#channel-name" - lookup in ChannelsInv
//   - "C1234567" - standard channel, lookup in Channels
//   - "G1234567" - private channel, lookup in Channels
//   - "D1234567" - DM channel, lookup in Channels
//   - "@username" - DM by username, lookup "@username" in ChannelsInv
func (tch *TeamContextHandler) resolveChannelInput(input string, channelsMap *provider.ChannelsCache) (id, displayName string, found bool) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", false
	}

	// Name-based lookup (starts with # or @)
	if strings.HasPrefix(input, "#") || strings.HasPrefix(input, "@") {
		if id, ok := channelsMap.ChannelsInv[input]; ok {
			if ch, ok := channelsMap.Channels[id]; ok {
				return id, ch.Name, true
			}
		}
		tch.logger.Warn("Channel not found by name", zap.String("input", input))
		return input, input, false
	}

	// ID-based lookup (C, G, or D prefix)
	if strings.HasPrefix(input, "C") || strings.HasPrefix(input, "G") || strings.HasPrefix(input, "D") {
		if ch, ok := channelsMap.Channels[input]; ok {
			return input, ch.Name, true
		}
		tch.logger.Warn("Channel not found by ID", zap.String("input", input))
		return input, input, false
	}

	tch.logger.Warn("Unknown channel format", zap.String("input", input))
	return input, input, false
}

// resolveUserInput resolves user reference to (id, displayName, found)
// Supported formats:
//   - "@username" - strip @ and lookup in UsersInv
//   - "U1234567" - standard user ID, lookup in Users
//   - "W1234567" - Enterprise Grid user ID, lookup in Users
func (tch *TeamContextHandler) resolveUserInput(input string, usersMap *provider.UsersCache) (id, displayName string, found bool) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", false
	}

	// Name-based lookup (starts with @)
	if strings.HasPrefix(input, "@") {
		username := strings.TrimPrefix(input, "@")
		if uid, ok := usersMap.UsersInv[username]; ok {
			if u, ok := usersMap.Users[uid]; ok {
				return uid, u.RealName, true
			}
		}
		tch.logger.Warn("User not found by name", zap.String("input", input))
		return input, input, false
	}

	// ID-based lookup (U or W prefix)
	if strings.HasPrefix(input, "U") || strings.HasPrefix(input, "W") {
		if u, ok := usersMap.Users[input]; ok {
			return input, u.RealName, true
		}
		tch.logger.Warn("User not found by ID", zap.String("input", input))
		return input, input, false
	}

	tch.logger.Warn("Unknown user format", zap.String("input", input))
	return input, input, false
}

// parseAliasEntry parses an entry that may contain an alias in format "alias=value" or just "value"
// Returns (alias, value) where alias may be empty if no alias was specified
func parseAliasEntry(entry string) (alias, value string) {
	entry = strings.TrimSpace(entry)
	if idx := strings.Index(entry, "="); idx != -1 {
		return strings.TrimSpace(entry[:idx]), strings.TrimSpace(entry[idx+1:])
	}
	return "", entry
}

// GetTeamContextHandler returns contextual information about priority channels and team members
func (tch *TeamContextHandler) GetTeamContextHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	tch.logger.Debug("GetTeamContextHandler called", zap.Any("params", request.Params))

	// Check if cache is ready
	ready, err := tch.apiProvider.IsReady()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to check cache readiness: %v", err)), nil
	}
	if !ready {
		return mcp.NewToolResultError("Slack workspace data is still loading. Please retry in a few seconds."), nil
	}

	// Read priority channels from env
	priorityChannels := os.Getenv("SLACK_MCP_PRIORITY_CHANNELS")
	priorityUsers := os.Getenv("SLACK_MCP_PRIORITY_USERS")
	teamName := os.Getenv("SLACK_MCP_TEAM_NAME")
	if teamName == "" {
		teamName = "your team"
	}

	// Build the context message
	var contextParts []string
	contextParts = append(contextParts, fmt.Sprintf("# Slack Workspace Context for %s\n", teamName))

	if priorityChannels != "" {
		contextParts = append(contextParts, "## Priority Channels")
		contextParts = append(contextParts, "These are the main channels to focus on. Use the channel_id shown when calling Slack tools:\n")

		channelEntries := strings.Split(priorityChannels, ",")
		channelsMap := tch.apiProvider.ProvideChannelsMaps()
		if channelsMap == nil || channelsMap.Channels == nil {
			return mcp.NewToolResultError("Channel cache not initialized"), nil
		}

		for _, entry := range channelEntries {
			entry = strings.TrimSpace(entry)
			if entry == "" {
				continue // Skip empty entries
			}
			alias, channelRef := parseAliasEntry(entry)
			id, name, found := tch.resolveChannelInput(channelRef, channelsMap)
			if found {
				if ch, ok := channelsMap.Channels[id]; ok {
					purpose := ch.Purpose
					if purpose == "" {
						purpose = "(no purpose set)"
					}
					if alias != "" {
						// Include alias mapping for Claude to understand
						contextParts = append(contextParts, fmt.Sprintf("- **%s** → #%s (channel_id: %s): %s", alias, name, id, purpose))
					} else {
						contextParts = append(contextParts, fmt.Sprintf("- #%s (channel_id: %s): %s", name, id, purpose))
					}
				}
			} else {
				contextParts = append(contextParts, fmt.Sprintf("- %s (WARNING: not found in workspace)", entry))
			}
		}
		contextParts = append(contextParts, "")
	}

	if priorityUsers != "" {
		contextParts = append(contextParts, "## Team Members")
		contextParts = append(contextParts, "These are the key team members. Use the @username or DM channel_id when calling Slack tools:\n")

		userEntries := strings.Split(priorityUsers, ",")
		usersMap := tch.apiProvider.ProvideUsersMap()
		channelsMap := tch.apiProvider.ProvideChannelsMaps()
		if usersMap == nil || usersMap.Users == nil {
			return mcp.NewToolResultError("User cache not initialized"), nil
		}

		for _, entry := range userEntries {
			entry = strings.TrimSpace(entry)
			if entry == "" {
				continue // Skip empty entries
			}
			alias, userRef := parseAliasEntry(entry)
			id, displayName, found := tch.resolveUserInput(userRef, usersMap)
			if found {
				if u, ok := usersMap.Users[id]; ok {
					// Try to find DM channel for this user
					dmChannelID := ""
					dmKey := "@" + u.Name
					if channelsMap != nil && channelsMap.ChannelsInv != nil {
						if dmID, ok := channelsMap.ChannelsInv[dmKey]; ok {
							dmChannelID = dmID
						}
					}

					if alias != "" {
						// Include alias mapping for Claude to understand
						if dmChannelID != "" {
							contextParts = append(contextParts, fmt.Sprintf("- **%s** → %s (@%s, user_id: %s, dm_channel: %s)", alias, displayName, u.Name, id, dmChannelID))
						} else {
							contextParts = append(contextParts, fmt.Sprintf("- **%s** → %s (@%s, user_id: %s)", alias, displayName, u.Name, id))
						}
					} else {
						if dmChannelID != "" {
							contextParts = append(contextParts, fmt.Sprintf("- %s (@%s, user_id: %s, dm_channel: %s)", displayName, u.Name, id, dmChannelID))
						} else {
							contextParts = append(contextParts, fmt.Sprintf("- %s (@%s, user_id: %s)", displayName, u.Name, id))
						}
					}
				}
			} else {
				contextParts = append(contextParts, fmt.Sprintf("- %s (WARNING: not found in workspace)", entry))
			}
		}
		contextParts = append(contextParts, "")
	}

	if priorityChannels != "" || priorityUsers != "" {
		contextParts = append(contextParts, "## Usage Guidelines")
		contextParts = append(contextParts, "- **IMPORTANT**: When the user mentions a person or channel by nickname/alias (shown in bold above), use the corresponding @username or channel_id in tool calls")
		contextParts = append(contextParts, "- For DMs with team members, use their dm_channel ID as channel_id in conversations_history")
		contextParts = append(contextParts, "- For user filters in search, use @username format (e.g., filter_users_from: '@i.bastos')")
		contextParts = append(contextParts, "- Bot messages are filtered by default. Use exclude_bots=false to include them.")
	} else {
		contextParts = append(contextParts, "No priority channels or team members configured. Set SLACK_MCP_PRIORITY_CHANNELS and SLACK_MCP_PRIORITY_USERS environment variables to provide context.")
	}

	return mcp.NewToolResultText(strings.Join(contextParts, "\n")), nil
}
