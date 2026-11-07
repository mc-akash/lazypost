package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

var version = "dev"

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "lazypost:", err)
		os.Exit(1)
	}
}

type options struct {
	workspace   string
	insecure    bool
	showVersion bool
	checkConfig bool
}

func parseFlags(args []string) (options, error) {
	var o options
	fs := flag.NewFlagSet("lazypost", flag.ContinueOnError)
	fs.StringVar(&o.workspace, "w", "", "workspace directory (default $LAZYPOST_HOME, config.yaml workspace, or ~/lazypost)")
	fs.StringVar(&o.workspace, "workspace", "", "workspace directory (default $LAZYPOST_HOME, config.yaml workspace, or ~/lazypost)")
	fs.BoolVar(&o.insecure, "insecure", false, "skip TLS certificate verification (also set by insecure in config.yaml)")
	fs.BoolVar(&o.showVersion, "version", false, "print version and exit")
	fs.BoolVar(&o.checkConfig, "check-config", false, "check config.yaml and exit")
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	if fs.NArg() > 0 {
		return o, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	return o, nil
}

func run(args []string, stdout io.Writer) error {
	o, err := parseFlags(args)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	if o.showVersion {
		fmt.Fprintln(stdout, "lazypost", version)
		return nil
	}
	b := newAppBuilder(o)
	if err := b.Init(initFromConfigFile); err != nil {
		return err
	}
	if o.checkConfig {
		if err := b.cfg.Keybinds.Validate(); err != nil {
			return fmt.Errorf("%s: %w", b.configPath, err)
		}
		fmt.Fprintln(stdout, b.configPath+": ok")
		return nil
	}
	r, err := b.Build()
	if err != nil {
		return err
	}
	return r.Run()
}
