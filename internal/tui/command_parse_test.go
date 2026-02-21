package tui

import (
	"strings"
	"testing"

	"helix-tui/internal/domain"
)

func TestParseCoreCommand(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		wantType  coreCommandType
		wantSide  domain.Side
		wantSym   string
		wantQty   float64
		wantErr   bool
		wantErrIn string
		wantNil   bool
	}{
		{name: "empty", raw: "", wantNil: true},
		{name: "quit", raw: "quit", wantType: coreCommandQuit},
		{name: "sync", raw: "sync", wantType: coreCommandSync},
		{name: "flatten", raw: "flatten", wantType: coreCommandFlatten},
		{name: "cancel", raw: "cancel abc", wantType: coreCommandCancel},
		{name: "buy", raw: "buy aapl 2", wantType: coreCommandOrder, wantSide: domain.SideBuy, wantSym: "AAPL", wantQty: 2},
		{name: "sell", raw: "sell msft 3", wantType: coreCommandOrder, wantSide: domain.SideSell, wantSym: "MSFT", wantQty: 3},
		{name: "cancel usage", raw: "cancel", wantErr: true, wantErrIn: "usage: cancel"},
		{name: "buy usage", raw: "buy aapl", wantErr: true, wantErrIn: "usage: buy"},
		{name: "qty invalid", raw: "buy aapl nope", wantErr: true, wantErrIn: "qty must"},
		{name: "unknown", raw: "wat", wantErr: true, wantErrIn: "unknown command"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, err := parseCoreCommand(tt.raw)
			if tt.wantNil {
				if cmd != nil || err != nil {
					t.Fatalf("expected nil command and nil err, got cmd=%#v err=%#v", cmd, err)
				}
				return
			}
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected parse error")
				}
				if !strings.Contains(err.status, tt.wantErrIn) {
					t.Fatalf("expected error %q to contain %q", err.status, tt.wantErrIn)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected parse error: %#v", err)
			}
			if cmd == nil {
				t.Fatalf("expected command")
			}
			if cmd.Type != tt.wantType {
				t.Fatalf("unexpected type: got %v want %v", cmd.Type, tt.wantType)
			}
			if tt.wantType == coreCommandOrder {
				if cmd.Side != tt.wantSide || cmd.Symbol != tt.wantSym || cmd.Qty != tt.wantQty {
					t.Fatalf("unexpected order parse: %#v", cmd)
				}
			}
		})
	}
}

func TestParseWatchCommand(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		wantSeen  bool
		wantType  watchCommandType
		wantSym   string
		wantErr   bool
		wantErrIn string
	}{
		{name: "non watch", raw: "help", wantSeen: false},
		{name: "list", raw: "watch", wantSeen: true, wantType: watchCommandList},
		{name: "list explicit", raw: "watch list", wantSeen: true, wantType: watchCommandList},
		{name: "sync", raw: "watch sync", wantSeen: true, wantType: watchCommandSync},
		{name: "pull alias", raw: "watch pull", wantSeen: true, wantType: watchCommandSync},
		{name: "add", raw: "watch add msft", wantSeen: true, wantType: watchCommandAdd, wantSym: "MSFT"},
		{name: "remove", raw: "watch remove aapl", wantSeen: true, wantType: watchCommandRemove, wantSym: "AAPL"},
		{name: "usage", raw: "watch add", wantSeen: true, wantErr: true, wantErrIn: "usage: watch"},
		{name: "unknown subcommand", raw: "watch nope aapl", wantSeen: true, wantErr: true, wantErrIn: "usage: watch"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, seen, err := parseWatchCommand(tt.raw)
			if seen != tt.wantSeen {
				t.Fatalf("unexpected handled: got %v want %v", seen, tt.wantSeen)
			}
			if !tt.wantSeen {
				if cmd != nil || err != nil {
					t.Fatalf("expected nil results for non-watch command, got cmd=%#v err=%#v", cmd, err)
				}
				return
			}
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected parse error")
				}
				if !strings.Contains(err.status, tt.wantErrIn) {
					t.Fatalf("expected error %q to contain %q", err.status, tt.wantErrIn)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected parse error: %#v", err)
			}
			if cmd == nil {
				t.Fatalf("expected command")
			}
			if cmd.Type != tt.wantType {
				t.Fatalf("unexpected type: got %v want %v", cmd.Type, tt.wantType)
			}
			if tt.wantSym != "" && cmd.Symbol != tt.wantSym {
				t.Fatalf("unexpected symbol: got %q want %q", cmd.Symbol, tt.wantSym)
			}
		})
	}
}

func TestParseEventsCommand(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		wantSeen  bool
		wantType  eventsCommandType
		wantStep  int
		wantErr   bool
		wantErrIn string
	}{
		{name: "non events", raw: "help", wantSeen: false},
		{name: "tail", raw: "events", wantSeen: true, wantType: eventsCommandTail},
		{name: "tail alias", raw: "events latest", wantSeen: true, wantType: eventsCommandTail},
		{name: "top", raw: "events top", wantSeen: true, wantType: eventsCommandTop},
		{name: "up default", raw: "events up", wantSeen: true, wantType: eventsCommandUp, wantStep: 8},
		{name: "down explicit", raw: "events down 3", wantSeen: true, wantType: eventsCommandDown, wantStep: 3},
		{name: "invalid n", raw: "events up nope", wantSeen: true, wantErr: true, wantErrIn: "usage: events"},
		{name: "unknown subcommand", raw: "events whatever", wantSeen: true, wantErr: true, wantErrIn: "usage: events"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, seen, err := parseEventsCommand(tt.raw, 8)
			if seen != tt.wantSeen {
				t.Fatalf("unexpected handled: got %v want %v", seen, tt.wantSeen)
			}
			if !tt.wantSeen {
				if cmd != nil || err != nil {
					t.Fatalf("expected nil results for non-events command, got cmd=%#v err=%#v", cmd, err)
				}
				return
			}
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected parse error")
				}
				if !strings.Contains(err.status, tt.wantErrIn) {
					t.Fatalf("expected error %q to contain %q", err.status, tt.wantErrIn)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected parse error: %#v", err)
			}
			if cmd == nil {
				t.Fatalf("expected command")
			}
			if cmd.Type != tt.wantType {
				t.Fatalf("unexpected type: got %v want %v", cmd.Type, tt.wantType)
			}
			if tt.wantStep > 0 && cmd.Step != tt.wantStep {
				t.Fatalf("unexpected step: got %d want %d", cmd.Step, tt.wantStep)
			}
		})
	}
}

func TestParseStrategyCommand(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		wantSeen  bool
		wantType  strategyCommandType
		wantID    uint
		wantErr   bool
		wantErrIn string
	}{
		{name: "non strategy", raw: "help", wantSeen: false},
		{name: "default status", raw: "strategy", wantSeen: true, wantType: strategyCommandStatus},
		{name: "run", raw: "strategy run", wantSeen: true, wantType: strategyCommandRun},
		{name: "status", raw: "strategy status", wantSeen: true, wantType: strategyCommandStatus},
		{name: "approve", raw: "strategy approve 12", wantSeen: true, wantType: strategyCommandApprove, wantID: 12},
		{name: "reject", raw: "strategy reject 9", wantSeen: true, wantType: strategyCommandReject, wantID: 9},
		{name: "archive", raw: "strategy archive 3", wantSeen: true, wantType: strategyCommandArchive, wantID: 3},
		{name: "invalid id", raw: "strategy approve nope", wantSeen: true, wantErr: true, wantErrIn: "strategy plan id must"},
		{name: "usage", raw: "strategy nope", wantSeen: true, wantErr: true, wantErrIn: "usage: strategy"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, seen, err := parseStrategyCommand(tt.raw)
			if seen != tt.wantSeen {
				t.Fatalf("unexpected handled: got %v want %v", seen, tt.wantSeen)
			}
			if !tt.wantSeen {
				if cmd != nil || err != nil {
					t.Fatalf("expected nil results for non-strategy command, got cmd=%#v err=%#v", cmd, err)
				}
				return
			}
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected parse error")
				}
				if !strings.Contains(err.status, tt.wantErrIn) {
					t.Fatalf("expected error %q to contain %q", err.status, tt.wantErrIn)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected parse error: %#v", err)
			}
			if cmd == nil {
				t.Fatalf("expected command")
			}
			if cmd.Type != tt.wantType {
				t.Fatalf("unexpected type: got %v want %v", cmd.Type, tt.wantType)
			}
			if tt.wantID > 0 && cmd.PlanID != tt.wantID {
				t.Fatalf("unexpected plan id: got %d want %d", cmd.PlanID, tt.wantID)
			}
		})
	}
}
