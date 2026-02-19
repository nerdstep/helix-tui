package runtime

import (
	"context"
	"fmt"
	"io"
)

func Run(ctx context.Context, args []string, stderr io.Writer) error {
	cfg, configPath, err := loadConfig(args)
	if err != nil {
		return err
	}
	options, err := parseRunOptions(args, cfg, configPath, stderr)
	if err != nil {
		return err
	}

	system, err := createSystem(options.cfg)
	if err != nil {
		return err
	}
	stopEventLogger, err := startEventFileLogger(ctx, system.Engine, options.cfg.LogFile, options.cfg.LogMode, stderr)
	if err != nil {
		return err
	}
	defer stopEventLogger()

	startRunner(ctx, system)

	if options.headless {
		RunHeadless(ctx, system.Engine)
		return nil
	}

	if err := runTUI(system, options.cfg); err != nil {
		return fmt.Errorf("runtime error: %w", err)
	}
	return nil
}
