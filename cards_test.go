package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestCardsCoverEveryOperationAndOutcome(t *testing.T) {
	tools := []string{"ubus_list", "ubus_call", "uci_get", "uci_apply", "uci_confirm", "exec", "logread", "wg_new_client", "mfa_unlock"}
	for _, tool := range tools {
		for _, outcome := range []Outcome{OutcomeOK, OutcomeDenied, OutcomeError} {
			t.Run(tool+"/"+string(outcome), func(t *testing.T) {
				res := textResult("fixture output")
				res.IsError = outcome != OutcomeOK
				attachOperationCard(res, tool, map[string]any{}, []string{"fixture"}, outcome, "fixture summary", 12)
				card := res.StructuredContent.(map[string]any)["card"].(operationCard)
				if card.Tool != tool || card.Outcome != outcome || card.Title == "" || card.Details == "" {
					t.Fatal("missing card fields", card)
				}
				if res.Meta["openai/outputTemplate"] != cardURI {
					t.Fatal("missing template")
				}
				if res.Content[0].(*mcp.TextContent).Text != "fixture output" {
					t.Fatal("text fallback changed")
				}
				if outcome != OutcomeOK && (card.Refresh != nil || card.Inspect != nil) {
					t.Fatal("error card can replay tool")
				}
			})
		}
	}
}

func TestCardRedactionAndMutationReplay(t *testing.T) {
	secret := "fixture-sensitive-value"
	fixtures := []struct {
		name, output string
		input        any
	}{
		{"uci_get", "wireless.radio.key='" + secret + "'\nnetwork.lan.proto='static'", uciGetIn{Config: "wireless"}},
		{"ubus_call", `{"password":"` + secret + `","nested":{"private_key":"` + secret + `","key":"` + secret + `"},"up":true}`, ubusCallIn{Object: "system", Method: "info"}},
		{"uci_apply", `ROLLBACK ARMED: this reverts automatically at 2026-10-08T15:00:00Z unless you call uci_confirm {"token":"` + secret + `"}`, uciApplyIn{Changes: []UCIChange{{Config: "wireless", Section: "radio", Option: "key", Value: secret}}}},
		{"uci_confirm", "Confirmed. " + secret, uciConfirmIn{Token: secret}},
		{"wg_new_client", "PrivateKey = " + secret + "\nQR with secret", map[string]any{}},
		{"mfa_unlock", "Unlocked.", mfaUnlockIn{Code: secret}},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			res := textResult(fixture.output)
			attachOperationCard(res, fixture.name, fixture.input, nil, OutcomeOK, "OK", 10)
			b, _ := json.Marshal(res.StructuredContent)
			if strings.Contains(string(b), secret) {
				t.Fatalf("secret in card: %s", b)
			}
			card := res.StructuredContent.(map[string]any)["card"].(operationCard)
			if fixture.name == "uci_apply" && (card.Refresh != nil || card.Inspect == nil || card.RollbackDeadline == "") {
				t.Fatal("invalid mutation controls", card)
			}
			if fixture.name == "uci_confirm" && card.Refresh != nil {
				t.Fatal("confirmation can replay")
			}
		})
	}
	for _, method := range []string{"reboot", "down", "renew", "up", "scan"} {
		res := textResult(`{"up":true}`)
		attachOperationCard(res, "ubus_call", ubusCallIn{Object: "system", Method: method}, nil, OutcomeOK, "OK", 1)
		if res.StructuredContent.(map[string]any)["card"].(operationCard).Refresh != nil {
			t.Fatal("unsafe ubus refresh", method)
		}
	}
}

func TestCardsSurviveMCPWireAndResources(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "config")
	if err := os.WriteFile(config, []byte("config server\n option audit '"+filepath.Join(dir, "audit")+"'\nconfig policy\n option client 'fixture'\n option tools 'logread'\n option scopes '*'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := NewServer(config, filepath.Join(dir, "state"))
	if err != nil {
		t.Fatal(err)
	}
	srv := s.newServerForClient("fixture")
	addTool(s, srv, "fixture", "logread", "fixture read", func(logreadIn) []string { return nil }, func(context.Context, logreadIn) (string, string, error) { return "fixture log", "one log", nil })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ct, st := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "card-test", Version: "1"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	catalog, err := cs.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Tools) != 10 {
		t.Fatal("unexpected tool count", len(catalog.Tools))
	}
	for _, tool := range catalog.Tools {
		if tool.Meta["openai/outputTemplate"] != cardURI || tool.Annotations == nil {
			t.Fatal("missing UI or annotation", tool.Name)
		}
	}
	for _, test := range []struct {
		name   string
		args   map[string]any
		denied bool
	}{{"logread", map[string]any{}, false}, {"exec", map[string]any{"argv": []string{"true"}}, true}} {
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: test.name, Arguments: test.args})
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError != test.denied || res.StructuredContent == nil {
			t.Fatal("card lost over wire", res)
		}
		b, _ := json.Marshal(res.StructuredContent)
		if !strings.Contains(string(b), `"tool":"`+test.name+`"`) {
			t.Fatal("bad structured card", string(b))
		}
	}
	resource, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: cardURI})
	if err != nil {
		t.Fatal(err)
	}
	if len(resource.Contents) != 1 || !strings.Contains(resource.Contents[0].Text, "OpenWrt operation") || resource.Contents[0].MIMEType != "text/html;profile=mcp-app" {
		t.Fatal("missing bundled resource")
	}
}

func TestSavedCardIsolationAndErrorDisplay(t *testing.T) {
	s := &Server{}
	r := errResult("denied: no grant")
	attachOperationCard(r, "exec", execIn{Argv: []string{"true"}}, nil, OutcomeDenied, "", 1)
	s.rememberCard("alice", r)
	shown := s.savedCard("alice", "exec")
	if shown.IsError || shown.StructuredContent.(map[string]any)["card"].(operationCard).Outcome != OutcomeDenied {
		t.Fatal("denied operation cannot be displayed")
	}
	if s.savedCard("bob", "exec").StructuredContent.(map[string]any)["card"].(operationCard).Outcome != OutcomeError {
		t.Fatal("cross-client result leak")
	}
	s.cards["alice"]["exec"] = operationCard{Tool: "exec", ObservedAt: time.Now().Add(-6 * time.Minute).Format(time.RFC3339)}
	if s.savedCard("alice", "exec").StructuredContent.(map[string]any)["card"].(operationCard).Outcome != OutcomeError {
		t.Fatal("expired result shown")
	}
}
