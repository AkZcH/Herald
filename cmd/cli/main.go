package main

import (
	"os"

	"github.com/AkZcH/herald/cmd/cli/commands"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	if err := commands.RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}