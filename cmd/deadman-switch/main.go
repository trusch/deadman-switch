package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/ghodss/yaml"
	"github.com/rs/zerolog/log"
	"github.com/spf13/pflag"
	"github.com/trusch/deadman-switch/pkg/config"
	"github.com/trusch/deadman-switch/pkg/server"
)

var (
	configFile      = pflag.StringP("config", "c", "config.yaml", "config file")
	showVersion     = pflag.BoolP("version", "v", false, "show version")
	Version, Commit string
)

func main() {
	ctx := context.Background()

	pflag.Parse()

	if *showVersion {
		fmt.Printf("Version: %s\nCommit: %s\n", Version, Commit)
		os.Exit(0)
	}

	cfg, err := loadConfig()
	if err != nil {
		log.Fatal().
			Err(err).
			Str("file", *configFile).
			Msg("failed to load config")
	}

	srv := server.New(ctx, cfg)

	err = srv.Listen(ctx)
	if err != nil {
		log.Fatal().
			Err(err).
			Msg("server stopped unexpectedly")
	}

}

func loadConfig() (cfg config.ServerConfig, err error) {
	bs, err := ioutil.ReadFile(*configFile)
	if err != nil {
		return cfg, err
	}
	err = yaml.Unmarshal(bs, &cfg)
	if err != nil {
		return cfg, err
	}
	return cfg, nil
}
