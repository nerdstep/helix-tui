package runtime

import (
	"context"
	"fmt"
	"io"
	"os"

	"helix-tui/internal/storage"
	"helix-tui/internal/version"
)

func Run(ctx context.Context, args []string, stderr io.Writer) error {
	options, err := parseRunOptions(args, stderr)
	if err != nil {
		return err
	}
	if options.version {
		_, _ = fmt.Fprintln(stderr, version.String())
		return nil
	}
	if err := maybeBootstrapConfig(options.configPath, options.configExplicit, os.Stdin, stderr); err != nil {
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
	if system.SettlementCalendar != nil {
		system.Engine.SetComplianceSettlementCalendar(system.SettlementCalendar)
		if cfg.ComplianceEnabled && cfg.ComplianceAvoidGoodFaith {
			system.Engine.AddEvent("compliance_calendar_source", "alpaca")
		}
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
			if system.StrategyRunner != nil {
				system.Runner.SetStrategyPolicyProvider(strategyPolicyAdapter{repo: store.Strategy()})
			}
		}
		if system.StrategyRunner != nil {
			system.StrategyRunner.SetStore(store.Strategy())
			system.StrategyRunner.SetEventHistory(store.Events())
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
