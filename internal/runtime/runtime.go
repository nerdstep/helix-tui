package runtime

import (
	"context"
	"fmt"
	"io"
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
	stopEventLogger, err := startEventFileLogger(ctx, system.Engine, cfg.LogFile, cfg.LogMode, cfg.LogLevel)
	if err != nil {
		return err
	}
	defer stopEventLogger()

	startRunner(ctx, system)

	if options.headless {
		RunHeadless(ctx, system.Engine)
		return nil
	}

	if err := runTUI(system, cfg); err != nil {
		return fmt.Errorf("runtime error: %w", err)
	}
	return nil
}
