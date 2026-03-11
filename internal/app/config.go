package app

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/skobkin/simple-hdd-tool/internal/domain"
)

const Version = "0.1.0"

type Config struct {
	GroupBy     domain.GroupMode
	SortBy      domain.SortMode
	ForceRemove bool
	NoColor     bool
	ShowHelp    bool
	ShowVersion bool
}

func ParseConfig(args []string) (Config, error) {
	cfg := Config{
		GroupBy: domain.GroupByModel,
		SortBy:  domain.SortBySize,
	}

	fs := flag.NewFlagSet("simple-hdd-tool", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var groupBy string
	var sortBy string
	fs.StringVar(&groupBy, "group-by", string(cfg.GroupBy), "")
	fs.StringVar(&sortBy, "sort-by", string(cfg.SortBy), "")
	fs.BoolVar(&cfg.ForceRemove, "force-remove", false, "")
	fs.BoolVar(&cfg.NoColor, "no-color", false, "")
	fs.BoolVar(&cfg.ShowHelp, "help", false, "")
	fs.BoolVar(&cfg.ShowVersion, "version", false, "")
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}

	switch domain.GroupMode(groupBy) {
	case domain.GroupByModel, domain.GroupBySize, domain.GroupByFamily, domain.GroupByNone:
		cfg.GroupBy = domain.GroupMode(groupBy)
	default:
		return cfg, fmt.Errorf("invalid --group-by value %q", groupBy)
	}

	switch domain.SortMode(sortBy) {
	case domain.SortBySize, domain.SortBySerial, domain.SortByHours:
		cfg.SortBy = domain.SortMode(sortBy)
	default:
		return cfg, fmt.Errorf("invalid --sort-by value %q", sortBy)
	}
	if cfg.ShowHelp || cfg.ShowVersion {
		return cfg, nil
	}
	if len(fs.Args()) > 0 {
		return cfg, errors.New("unexpected positional arguments")
	}
	return cfg, nil
}

func HelpText() string {
	return `simple-hdd-tool

Usage:
  simple-hdd-tool [--group-by=model|size|family|none] [--sort-by=size|serial|hours] [--force-remove] [--no-color] [--version]
`
}
