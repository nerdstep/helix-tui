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
	coreCommandHelp
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
)

type strategyCommand struct {
	Type strategyCommandType
}

func parseCoreCommand(raw string) (*coreCommand, *statusOnlyMsg) {
	args := strings.Fields(raw)
	if len(args) == 0 {
		return nil, nil
	}

	switch strings.ToLower(args[0]) {
	case "q", "quit", "exit":
		return &coreCommand{Type: coreCommandQuit}, nil
	case "help":
		return &coreCommand{Type: coreCommandHelp}, nil
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
		return nil, statusError("unknown command; type help")
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
	args := lowerCommandArgs(raw)
	if len(args) == 0 || args[0] != "strategy" {
		return nil, false, nil
	}
	if len(args) == 1 {
		return &strategyCommand{Type: strategyCommandStatus}, true, nil
	}
	if len(args) != 2 {
		return nil, true, statusError(strategyCommandUsage)
	}
	switch args[1] {
	case "run":
		return &strategyCommand{Type: strategyCommandRun}, true, nil
	case "status":
		return &strategyCommand{Type: strategyCommandStatus}, true, nil
	default:
		return nil, true, statusError(strategyCommandUsage)
	}
}
