package tui

import (
	"strconv"
	"strings"

	"helix-tui/internal/domain"
	"helix-tui/internal/symbols"
)

type coreCommandType uint8

const (
	coreCommandQuit coreCommandType = iota + 1
	coreCommandSync
	coreCommandFlatten
	coreCommandCancel
	coreCommandOrder
)

type coreCommand struct {
	Type    coreCommandType
	OrderID string
	Symbol  string
	Side    domain.Side
	Qty     float64
}

type watchCommandType uint8

const (
	watchCommandList watchCommandType = iota + 1
	watchCommandSync
	watchCommandAdd
	watchCommandRemove
)

type watchCommand struct {
	Type   watchCommandType
	Symbol string
}

type eventsCommandType uint8

const (
	eventsCommandTail eventsCommandType = iota + 1
	eventsCommandTop
	eventsCommandUp
	eventsCommandDown
)

type eventsCommand struct {
	Type eventsCommandType
	Step int
}

type strategyCommandType uint8

const (
	strategyCommandRun strategyCommandType = iota + 1
	strategyCommandStatus
	strategyCommandApprove
	strategyCommandReject
	strategyCommandArchive
	strategyCommandProposalStatus
	strategyCommandProposalList
	strategyCommandProposalApply
	strategyCommandProposalReject
	strategyCommandChatStatus
	strategyCommandChatList
	strategyCommandChatNew
	strategyCommandChatUse
	strategyCommandChatSay
)

type strategyCommand struct {
	Type     strategyCommandType
	PlanID   uint
	ThreadID uint
	Text     string
}

func parseCoreCommand(raw string) (*coreCommand, *statusOnlyMsg) {
	args := strings.Fields(raw)
	if len(args) == 0 {
		return nil, nil
	}

	switch strings.ToLower(args[0]) {
	case "q", "quit", "exit":
		return &coreCommand{Type: coreCommandQuit}, nil
	case "sync":
		return &coreCommand{Type: coreCommandSync}, nil
	case "flatten":
		return &coreCommand{Type: coreCommandFlatten}, nil
	case "cancel":
		if len(args) != 2 {
			return nil, statusError("usage: cancel <ORDER_ID|ORDER_ID_PREFIX|#ROW>")
		}
		return &coreCommand{Type: coreCommandCancel, OrderID: args[1]}, nil
	case "buy", "sell":
		if len(args) != 3 {
			return nil, statusError("usage: " + strings.ToLower(args[0]) + " <SYM> <QTY>")
		}
		qty, err := strconv.ParseFloat(args[2], 64)
		if err != nil || qty <= 0 {
			return nil, statusError("qty must be a positive number")
		}
		side := domain.SideBuy
		if strings.EqualFold(args[0], "sell") {
			side = domain.SideSell
		}
		return &coreCommand{
			Type:   coreCommandOrder,
			Symbol: strings.ToUpper(args[1]),
			Side:   side,
			Qty:    qty,
		}, nil
	default:
		return nil, statusError("unknown command; use ? for help")
	}
}

func parseWatchCommand(raw string) (*watchCommand, bool, *statusOnlyMsg) {
	args := lowerCommandArgs(raw)
	if len(args) == 0 || args[0] != "watch" {
		return nil, false, nil
	}
	if len(args) == 1 || args[1] == "list" {
		return &watchCommand{Type: watchCommandList}, true, nil
	}
	if len(args) == 2 && (args[1] == "sync" || args[1] == "pull") {
		return &watchCommand{Type: watchCommandSync}, true, nil
	}
	if len(args) != 3 {
		return nil, true, statusError(watchCommandUsage)
	}

	normalized := symbols.Normalize([]string{args[2]})
	if len(normalized) == 0 {
		return nil, true, statusError("symbol is required")
	}
	symbol := normalized[0]

	switch args[1] {
	case "add":
		return &watchCommand{Type: watchCommandAdd, Symbol: symbol}, true, nil
	case "remove":
		return &watchCommand{Type: watchCommandRemove, Symbol: symbol}, true, nil
	default:
		return nil, true, statusError(watchCommandUsage)
	}
}

func parseEventsCommand(raw string, defaultStep int) (*eventsCommand, bool, *statusOnlyMsg) {
	args := lowerCommandArgs(raw)
	if len(args) == 0 || args[0] != "events" {
		return nil, false, nil
	}
	if len(args) == 1 || args[1] == "tail" || args[1] == "latest" || args[1] == "end" {
		return &eventsCommand{Type: eventsCommandTail}, true, nil
	}
	if args[1] == "top" || args[1] == "oldest" {
		return &eventsCommand{Type: eventsCommandTop}, true, nil
	}

	step, ok := parseEventStep(args, defaultStep)
	if !ok {
		return nil, true, statusError(eventsCommandUsage)
	}
	switch args[1] {
	case "up", "older", "back":
		return &eventsCommand{Type: eventsCommandUp, Step: step}, true, nil
	case "down", "newer", "forward":
		return &eventsCommand{Type: eventsCommandDown, Step: step}, true, nil
	default:
		return nil, true, statusError(eventsCommandUsage)
	}
}

func parseStrategyCommand(raw string) (*strategyCommand, bool, *statusOnlyMsg) {
	raw = strings.TrimSpace(raw)
	args := strings.Fields(raw)
	if len(args) == 0 || !strings.EqualFold(args[0], "strategy") {
		return nil, false, nil
	}
	lower := make([]string, 0, len(args))
	for _, arg := range args {
		lower = append(lower, strings.ToLower(strings.TrimSpace(arg)))
	}
	if len(lower) == 1 {
		return &strategyCommand{Type: strategyCommandStatus}, true, nil
	}
	if lower[1] == "proposal" || lower[1] == "proposals" {
		return parseStrategyProposalCommand(lower)
	}
	if lower[1] == "chat" {
		return parseStrategyChatCommand(raw, lower)
	}
	if len(lower) == 2 {
		switch lower[1] {
		case "run":
			return &strategyCommand{Type: strategyCommandRun}, true, nil
		case "status":
			return &strategyCommand{Type: strategyCommandStatus}, true, nil
		default:
			return nil, true, statusError(strategyCommandUsage)
		}
	}
	if len(lower) != 3 {
		return nil, true, statusError(strategyCommandUsage)
	}
	planID64, err := strconv.ParseUint(strings.TrimSpace(lower[2]), 10, 64)
	if err != nil || planID64 == 0 {
		return nil, true, statusError("strategy plan id must be a positive integer")
	}
	planID := uint(planID64)
	switch lower[1] {
	case "approve", "activate":
		return &strategyCommand{Type: strategyCommandApprove, PlanID: planID}, true, nil
	case "reject", "supersede":
		return &strategyCommand{Type: strategyCommandReject, PlanID: planID}, true, nil
	case "archive":
		return &strategyCommand{Type: strategyCommandArchive, PlanID: planID}, true, nil
	default:
		return nil, true, statusError(strategyCommandUsage)
	}
}

func parseStrategyProposalCommand(lower []string) (*strategyCommand, bool, *statusOnlyMsg) {
	if len(lower) == 2 {
		return &strategyCommand{Type: strategyCommandProposalStatus}, true, nil
	}
	if len(lower) == 3 {
		switch lower[2] {
		case "status":
			return &strategyCommand{Type: strategyCommandProposalStatus}, true, nil
		case "list":
			return &strategyCommand{Type: strategyCommandProposalList}, true, nil
		default:
			return nil, true, statusError(strategyProposalCommandUsage)
		}
	}
	if len(lower) != 4 {
		return nil, true, statusError(strategyProposalCommandUsage)
	}
	proposalID64, err := strconv.ParseUint(strings.TrimSpace(lower[3]), 10, 64)
	if err != nil || proposalID64 == 0 {
		return nil, true, statusError("strategy proposal id must be a positive integer")
	}
	proposalID := uint(proposalID64)
	switch lower[2] {
	case "apply":
		return &strategyCommand{Type: strategyCommandProposalApply, PlanID: proposalID}, true, nil
	case "reject":
		return &strategyCommand{Type: strategyCommandProposalReject, PlanID: proposalID}, true, nil
	default:
		return nil, true, statusError(strategyProposalCommandUsage)
	}
}

func parseStrategyChatCommand(raw string, lower []string) (*strategyCommand, bool, *statusOnlyMsg) {
	if len(lower) == 2 {
		return &strategyCommand{Type: strategyCommandChatStatus}, true, nil
	}
	if len(lower) == 3 {
		switch lower[2] {
		case "status":
			return &strategyCommand{Type: strategyCommandChatStatus}, true, nil
		case "list":
			return &strategyCommand{Type: strategyCommandChatList}, true, nil
		case "new":
			return nil, true, statusError("strategy chat title is required")
		case "say", "ask":
			return nil, true, statusError("strategy chat message is required")
		default:
			return nil, true, statusError(strategyChatCommandUsage)
		}
	}
	switch lower[2] {
	case "new":
		title := commandTailAfterNFields(raw, 3)
		if title == "" {
			return nil, true, statusError("strategy chat title is required")
		}
		return &strategyCommand{Type: strategyCommandChatNew, Text: title}, true, nil
	case "use", "select":
		if len(lower) != 4 {
			return nil, true, statusError(strategyChatCommandUsage)
		}
		threadID64, err := strconv.ParseUint(strings.TrimSpace(lower[3]), 10, 64)
		if err != nil || threadID64 == 0 {
			return nil, true, statusError("strategy chat thread id must be a positive integer")
		}
		return &strategyCommand{Type: strategyCommandChatUse, ThreadID: uint(threadID64)}, true, nil
	case "say", "ask":
		text := commandTailAfterNFields(raw, 3)
		if text == "" {
			return nil, true, statusError("strategy chat message is required")
		}
		return &strategyCommand{Type: strategyCommandChatSay, Text: text}, true, nil
	default:
		return nil, true, statusError(strategyChatCommandUsage)
	}
}

func commandTailAfterNFields(raw string, fields int) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || fields <= 0 {
		return ""
	}
	start := 0
	inToken := false
	consumed := 0
	for i, r := range raw {
		isSpace := r == ' ' || r == '\t' || r == '\n' || r == '\r'
		if !inToken {
			if isSpace {
				continue
			}
			inToken = true
			consumed++
			if consumed == fields+1 {
				start = i
			}
			continue
		}
		if isSpace {
			inToken = false
		}
	}
	if consumed <= fields || start >= len(raw) {
		return ""
	}
	return strings.TrimSpace(raw[start:])
}
