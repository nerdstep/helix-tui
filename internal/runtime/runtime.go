package runtime

import (
	"context"
	"fmt"
	"io"

	"helix-tui/internal/storage"
)

func Run(ctx context.Context, args []string, stderr io.Writer) error {
	options, err := parseRunOptions(args, stderr)
	if err != nil {
		return err
	}
	cfg, err := loadConfig(options.configPath, options.configExplicit)
	if err != nil {
		return err
	}

	system, err := createSystem(cfg)
	if err != nil {
		return err
	}

	var store *storage.Store
	store, err = storage.Open(storage.Config{Path: cfg.DatabasePath})
	if err != nil {
		system.Engine.AddEvent("database_error", err.Error())
	} else {
		if setErr := system.Engine.SetComplianceSettlementStore(complianceStateAdapter{repo: store.ComplianceState()}); setErr != nil {
			system.Engine.AddEvent("database_error", fmt.Sprintf("load compliance settlement state: %v", setErr))
		}
		persistor := newTradeEventPersistor(store.Events(), system.Engine.AddStructuredEvent)
		system.Engine.AddEventSink(persistor.HandleEvent)
		defer persistor.Close()

		defer func() {
			if closeErr := store.Close(); closeErr != nil {
				system.Engine.AddEvent("database_error", fmt.Sprintf("close: %v", closeErr))
			}
		}()
		if system.Runner != nil {
			system.Runner.SetEventHistory(store.Events())
		}
	}

	stopEventLogger, err := startEventFileLogger(ctx, system.Engine, cfg.LogFile, cfg.LogMode, cfg.LogLevel)
	if err != nil {
		return err
	}
	defer stopEventLogger()

	updateQuoteStream := startQuoteStreaming(ctx, system)
	startRunner(ctx, system)

	if options.headless {
		RunHeadless(ctx, system.Engine)
		return nil
	}

	if err := runTUI(system, store, updateQuoteStream); err != nil {
		return fmt.Errorf("runtime error: %w", err)
	}
	return nil
}
