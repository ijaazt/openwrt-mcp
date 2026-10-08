package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const cardURI = "ui://openwrt/operation-card-v1.html"

// Embedded at compile time: no Node runtime, external assets or credentials on the router.
//
//go:embed ui/card.html
var cardHTML string

func cardToolMeta() mcp.Meta {
	return mcp.Meta{
		"ui":                      map[string]any{"resourceUri": cardURI, "visibility": []string{"model", "app"}},
		"openai/outputTemplate":   cardURI,
		"openai/widgetAccessible": true,
	}
}

func registerCardResource(srv *mcp.Server) {
	meta := mcp.Meta{
		"ui": map[string]any{"prefersBorder": true, "csp": map[string]any{
			"connectDomains": []string{}, "resourceDomains": []string{},
		}},
		"openai/widgetDescription":   "OpenWrt operation result: router status, configuration, logs, permission errors or rollback status. Refresh only repeats known diagnostic reads; never repeats a write.",
		"openai/widgetPrefersBorder": true,
		"openai/widgetCSP":           map[string]any{"connect_domains": []string{}, "resource_domains": []string{}},
		"openai/ui":                  map[string]any{"availableDisplayModes": []string{"inline"}, "preferredDisplayMode": "inline"},
	}
	srv.AddResource(&mcp.Resource{Name: "openwrt-operation-card", Title: "OpenWrt operation", URI: cardURI,
		MIMEType: "text/html;profile=mcp-app", Meta: meta},
		func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: cardURI,
				MIMEType: "text/html;profile=mcp-app", Text: cardHTML, Meta: meta}}}, nil
		})
}

func cardTitle(tool string) string {
	titles := map[string]string{
		"ubus_list": "Router capabilities", "ubus_call": "Router status", "uci_get": "Router configuration",
		"uci_apply": "Configuration change", "uci_confirm": "Change confirmation", "logread": "Router logs",
		"exec": "Router command", "wg_new_client": "WireGuard client", "mfa_unlock": "Second-factor access",
	}
	if title := titles[tool]; title != "" {
		return title
	}
	return tool
}

func cardAnnotations(tool string) *mcp.ToolAnnotations {
	read := tool == "ubus_list" || tool == "uci_get" || tool == "logread"
	destructive, open := !read, tool == "exec"
	return &mcp.ToolAnnotations{ReadOnlyHint: read, DestructiveHint: &destructive,
		IdempotentHint: read, OpenWorldHint: &open}
}

type cardRead struct {
	Name      string `json:"name"`
	Arguments any    `json:"arguments"`
}

type operationCard struct {
	Tool             string    `json:"tool"`
	Title            string    `json:"title"`
	Outcome          Outcome   `json:"outcome"`
	Summary          string    `json:"summary"`
	Scope            []string  `json:"scope"`
	ObservedAt       string    `json:"observed_at"`
	DurationMS       int64     `json:"duration_ms"`
	Details          string    `json:"details"`
	Data             any       `json:"data,omitempty"`
	Changes          any       `json:"changes,omitempty"`
	Refresh          *cardRead `json:"refresh,omitempty"`
	Inspect          *cardRead `json:"inspect,omitempty"`
	RollbackDeadline string    `json:"rollback_deadline,omitempty"`
	Notice           string    `json:"notice,omitempty"`
}

// Extra card redaction handles UCI's bare key option and TOTP codes. Never mirror
// the private WireGuard config/QR or confirmation tokens into the HTML payload.
func cardSecretKey(k string) bool {
	lk := strings.ToLower(k)
	return isSecretKey(k) || lk == "key" || strings.HasSuffix(lk, ".key") || lk == "code"
}

func cardRedact(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, value := range t {
			if cardSecretKey(k) {
				out[k] = redacted
			} else {
				out[k] = cardRedact(value)
			}
		}
		// UCI change payloads use {option: "key", value: "..."}.
		if option, ok := t["option"].(string); ok && cardSecretKey(option) {
			out["value"] = redacted
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, value := range t {
			out[i] = cardRedact(value)
		}
		return out
	default:
		return v
	}
}

var cardAssignment = regexp.MustCompile(`(?m)^([^\n=:]+)([=:])(.*)$`)
var cardJSONToken = regexp.MustCompile(`(?i)("(?:token|password|privatekey|private_key|code)"\s*:\s*")[^"]*(")`)
var cardDeadline = regexp.MustCompile(`reverts automatically at ([0-9TZ:+\-]+)`)

func redactCardText(value string) string {
	value = cardAssignment.ReplaceAllStringFunc(value, func(line string) string {
		m := cardAssignment.FindStringSubmatch(line)
		if cardSecretKey(strings.TrimSpace(m[1])) {
			return m[1] + m[2] + redacted
		}
		return line
	})
	return cardJSONToken.ReplaceAllString(value, `${1}<redacted>${2}`)
}

func safeUbusRead(object, method string) bool {
	switch object {
	case "system":
		return method == "board" || method == "info"
	case "network.interface":
		return method == "dump" || method == "status"
	case "network.wireless":
		return method == "status" || method == "get_validate"
	case "luci-rpc":
		return method == "getNetworkDevices" || method == "getWirelessDevices" || method == "getHostHints" || method == "getDUIDHints" || method == "getBoardJSON" || method == "getDHCPLeases"
	case "iwinfo":
		return method == "devices" || method == "info" || method == "assoclist" || method == "freqlist" || method == "txpowerlist" || method == "countrylist" || method == "survey" || method == "phyname"
	}
	return strings.HasPrefix(object, "network.interface.") && method == "status"
}

func attachOperationCard(result *mcp.CallToolResult, name string, input any, scopes []string, outcome Outcome, summary string, duration int64) {
	card := operationCard{Tool: name, Title: cardTitle(name), Outcome: outcome, Summary: summary,
		Scope: scopes, ObservedAt: nowISO(), DurationMS: duration}
	for _, content := range result.Content {
		if text, ok := content.(*mcp.TextContent); ok {
			card.Details += text.Text + "\n"
		}
	}
	card.Details = strings.TrimSpace(card.Details)
	if summary == "" {
		card.Summary = string(outcome)
	}
	card.Summary = redactCardText(card.Summary)
	if name == "uci_confirm" {
		card.Summary = "Confirmation result"
	}
	// Parse the bounded ubus JSON prefix; keep upstream's pruning notice visible.
	parts := strings.SplitN(card.Details, "\n\n[pruned:", 2)
	body := parts[0]
	var data any
	if json.Unmarshal([]byte(body), &data) == nil {
		card.Data = cardRedact(data)
		if b, err := json.MarshalIndent(card.Data, "", "  "); err == nil {
			card.Details = string(b)
			if len(parts) == 2 {
				card.Details += "\n\n[pruned:" + parts[1]
			}
		}
	} else {
		card.Details = redactCardText(card.Details)
	}
	var args map[string]any
	if b, err := json.Marshal(input); err == nil {
		_ = json.Unmarshal(b, &args)
	}
	if outcome == OutcomeOK {
		switch name {
		case "ubus_list", "uci_get", "logread":
			card.Refresh = &cardRead{Name: name, Arguments: args}
		case "ubus_call":
			object, _ := args["object"].(string)
			method, _ := args["method"].(string)
			if safeUbusRead(object, method) {
				card.Refresh = &cardRead{Name: name, Arguments: args}
			}
		case "uci_apply":
			card.Changes = cardRedact(args["changes"])
			if m := cardDeadline.FindStringSubmatch(card.Details); len(m) == 2 {
				if _, err := time.Parse(time.RFC3339, m[1]); err == nil {
					card.RollbackDeadline = m[1]
				}
			}
			card.Notice = "Verify a fresh connection and the intended behavior before confirming. This card never confirms or repeats a change automatically."
			if changes, ok := args["changes"].([]any); ok && len(changes) > 0 {
				if change, ok := changes[0].(map[string]any); ok {
					card.Inspect = &cardRead{Name: "uci_get", Arguments: map[string]any{"config": change["config"]}}
				}
			}
		case "wg_new_client":
			card.Details = "Client created. The credential-bearing configuration and QR remain in the original tool output."
			card.Data = nil
			card.Notice = "Treat the original private key as a credential. It is not copied into this card."
		}
	}
	if name == "mfa_unlock" {
		card.Notice = "Access expires automatically. Your authentication code is never included in this card."
	}
	if in, ok := input.(uciConfirmIn); ok && in.Token != "" {
		card.Details = strings.ReplaceAll(card.Details, in.Token, redacted)
	}
	result.StructuredContent = map[string]any{"card": card}
	result.Meta = cardToolMeta()
}
