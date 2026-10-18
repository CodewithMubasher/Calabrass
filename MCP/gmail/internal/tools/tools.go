// Package tools provides the Gmail tools for the MCP server.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/CodewithMubasher/Calabrass/mcp/gmail/internal/gmail"
	"github.com/CodewithMubasher/Calabrass/mcp/gmail/internal/protocol"
)

// Instructions returns the server instructions for the model.
func Instructions() string {
	return `This server provides access to Gmail through Google's official Gmail API.

Available tools:
- search_emails: Search for emails with Gmail query syntax
- read_email: Read a specific email by ID
- send_email: Send a new email
- create_draft: Create a draft email
- list_drafts: List draft emails
- modify_labels: Add or remove labels from an email
- list_labels: List all Gmail labels

The server uses OAuth2 authentication. On first tool call, it will open a browser for Google authorization if needed.`
}

// All returns all Gmail tools.
func All(client *gmail.Client) []protocol.Tool {
	return []protocol.Tool{
		searchEmailsTool(client),
		readEmailTool(client),
		sendEmailTool(client),
		createDraftTool(client),
		listDraftsTool(client),
		modifyLabelsTool(client),
		listLabelsTool(client),
	}
}

// searchEmailsTool searches for emails using Gmail query syntax.
func searchEmailsTool(client *gmail.Client) protocol.Tool {
	return protocol.Tool{
		Name:        "search_emails",
		Description: "Search for emails using Gmail query syntax (e.g., 'from:example@gmail.com', 'subject:invoice', 'is:unread', 'has:attachment'). Returns a list of matching emails with subject, sender, date, and snippet.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Gmail search query (same syntax as Gmail search box)",
				},
				"max_results": map[string]any{
					"type":        "integer",
					"description": "Maximum number of results to return (default: 10, max: 50)",
					"default":     10,
					"minimum":     1,
					"maximum":     50,
				},
			},
			"required": []string{"query"},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			query, ok := args["query"].(string)
			if !ok || query == "" {
				return "", fmt.Errorf("query is required")
			}

			maxResults := int64(10)
			if mr, ok := args["max_results"].(float64); ok {
				maxResults = int64(mr)
			}
			if maxResults > 50 {
				maxResults = 50
			}

			messages, err := client.ListMessages(ctx, query, maxResults)
			if err != nil {
				return "", fmt.Errorf("search failed: %w", err)
			}

			if len(messages) == 0 {
				return "No emails found matching query: " + query, nil
			}

			var results []map[string]any
			for _, msg := range messages {
				results = append(results, map[string]any{
					"id":        msg.ID,
					"thread_id": msg.ThreadID,
					"subject":   msg.Subject,
					"from":      msg.From,
					"to":        msg.To,
					"date":      msg.Date,
					"snippet":   msg.Snippet,
					"labels":    msg.Labels,
				})
			}

			data, _ := json.MarshalIndent(results, "", "  ")
			return string(data), nil
		},
	}
}

// readEmailTool reads a specific email by ID.
func readEmailTool(client *gmail.Client) protocol.Tool {
	return protocol.Tool{
		Name:        "read_email",
		Description: "Read the full content of a specific email by its ID. Returns subject, sender, recipients, date, labels, and full body.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{
					"type":        "string",
					"description": "The email message ID (from search_emails results)",
				},
			},
			"required": []string{"id"},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			id, ok := args["id"].(string)
			if !ok || id == "" {
				return "", fmt.Errorf("email ID is required")
			}

			msg, err := client.GetMessage(ctx, id)
			if err != nil {
				return "", fmt.Errorf("failed to read email: %w", err)
			}

			result := map[string]any{
				"id":        msg.ID,
				"thread_id": msg.ThreadID,
				"subject":   msg.Subject,
				"from":      msg.From,
				"to":        msg.To,
				"date":      msg.Date,
				"labels":    msg.Labels,
				"snippet":   msg.Snippet,
				"body":      msg.Body,
			}

			data, _ := json.MarshalIndent(result, "", "  ")
			return string(data), nil
		},
	}
}

// sendEmailTool sends a new email.
func sendEmailTool(client *gmail.Client) protocol.Tool {
	return protocol.Tool{
		Name:        "send_email",
		Description: "Send a new email. Requires recipient, subject, and body.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"to": map[string]any{
					"type":        "string",
					"description": "Recipient email address",
				},
				"subject": map[string]any{
					"type":        "string",
					"description": "Email subject",
				},
				"body": map[string]any{
					"type":        "string",
					"description": "Email body (plain text)",
				},
			},
			"required": []string{"to", "subject", "body"},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			to, _ := args["to"].(string)
			subject, _ := args["subject"].(string)
			body, _ := args["body"].(string)

			if to == "" || subject == "" || body == "" {
				return "", fmt.Errorf("to, subject, and body are required")
			}

			if err := client.SendMessage(ctx, to, subject, body); err != nil {
				return "", fmt.Errorf("failed to send email: %w", err)
			}

			return fmt.Sprintf("Email sent successfully to %s", to), nil
		},
	}
}

// createDraftTool creates a draft email.
func createDraftTool(client *gmail.Client) protocol.Tool {
	return protocol.Tool{
		Name:        "create_draft",
		Description: "Create a draft email. Requires recipient, subject, and body.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"to": map[string]any{
					"type":        "string",
					"description": "Recipient email address",
				},
				"subject": map[string]any{
					"type":        "string",
					"description": "Email subject",
				},
				"body": map[string]any{
					"type":        "string",
					"description": "Email body (plain text)",
				},
			},
			"required": []string{"to", "subject", "body"},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			to, _ := args["to"].(string)
			subject, _ := args["subject"].(string)
			body, _ := args["body"].(string)

			if to == "" || subject == "" || body == "" {
				return "", fmt.Errorf("to, subject, and body are required")
			}

			draft, err := client.CreateDraft(ctx, to, subject, body)
			if err != nil {
				return "", fmt.Errorf("failed to create draft: %w", err)
			}

			return fmt.Sprintf("Draft created successfully (ID: %s)", draft.ID), nil
		},
	}
}

// listDraftsTool lists draft emails.
func listDraftsTool(client *gmail.Client) protocol.Tool {
	return protocol.Tool{
		Name:        "list_drafts",
		Description: "List draft emails.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"max_results": map[string]any{
					"type":        "integer",
					"description": "Maximum number of drafts to return (default: 20, max: 100)",
					"default":     20,
					"minimum":     1,
					"maximum":     100,
				},
			},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			maxResults := int64(20)
			if mr, ok := args["max_results"].(float64); ok {
				maxResults = int64(mr)
			}
			if maxResults > 100 {
				maxResults = 100
			}

			drafts, err := client.ListDrafts(ctx, maxResults)
			if err != nil {
				return "", fmt.Errorf("failed to list drafts: %w", err)
			}

			if len(drafts) == 0 {
				return "No drafts found", nil
			}

			var results []map[string]any
			for _, d := range drafts {
				results = append(results, map[string]any{"id": d.ID})
			}

			data, _ := json.MarshalIndent(results, "", "  ")
			return string(data), nil
		},
	}
}

// modifyLabelsTool adds or removes labels from an email.
func modifyLabelsTool(client *gmail.Client) protocol.Tool {
	return protocol.Tool{
		Name:        "modify_labels",
		Description: "Add or remove labels from an email. Use label names like 'INBOX', 'UNREAD', 'STARRED', 'IMPORTANT', 'CATEGORY_PERSONAL', etc. To mark as read, remove 'UNREAD'. To archive, remove 'INBOX'.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{
					"type":        "string",
					"description": "Email message ID",
				},
				"add_labels": map[string]any{
					"type":        "array",
					"description": "Label names to add",
					"items":       map[string]any{"type": "string"},
				},
				"remove_labels": map[string]any{
					"type":        "array",
					"description": "Label names to remove",
					"items":       map[string]any{"type": "string"},
				},
			},
			"required": []string{"id"},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			id, _ := args["id"].(string)
			if id == "" {
				return "", fmt.Errorf("email ID is required")
			}

			var addLabels, removeLabels []string
			if al, ok := args["add_labels"].([]any); ok {
				for _, l := range al {
					if s, ok := l.(string); ok {
						addLabels = append(addLabels, s)
					}
				}
			}
			if rl, ok := args["remove_labels"].([]any); ok {
				for _, l := range rl {
					if s, ok := l.(string); ok {
						removeLabels = append(removeLabels, s)
					}
				}
			}

			if len(addLabels) == 0 && len(removeLabels) == 0 {
				return "", fmt.Errorf("at least one add_labels or remove_labels is required")
			}

			if err := client.ModifyLabels(ctx, id, addLabels, removeLabels); err != nil {
				return "", fmt.Errorf("failed to modify labels: %w", err)
			}

			var actions []string
			if len(addLabels) > 0 {
				actions = append(actions, "added: "+strings.Join(addLabels, ", "))
			}
			if len(removeLabels) > 0 {
				actions = append(actions, "removed: "+strings.Join(removeLabels, ", "))
			}
			return fmt.Sprintf("Labels updated for email %s: %s", id, strings.Join(actions, "; ")), nil
		},
	}
}

// listLabelsTool lists all Gmail labels.
func listLabelsTool(client *gmail.Client) protocol.Tool {
	return protocol.Tool{
		Name:        "list_labels",
		Description: "List all Gmail labels available for the account.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			labels, err := client.ListLabels(ctx)
			if err != nil {
				return "", fmt.Errorf("failed to list labels: %w", err)
			}

			if len(labels) == 0 {
				return "No labels found", nil
			}

			var results []map[string]any
			for _, label := range labels {
				results = append(results, map[string]any{
					"id":               label.Id,
					"name":             label.Name,
					"type":             label.Type,
					"messages_total":   label.MessagesTotal,
					"messages_unread":  label.MessagesUnread,
					"threads_total":    label.ThreadsTotal,
					"threads_unread":   label.ThreadsUnread,
				})
			}

			data, _ := json.MarshalIndent(results, "", "  ")
			return string(data), nil
		},
	}
}