package main

import (
	"github.com/permgps/herdr-telegram-agents/internal/cli"
	"os"
)

func main() { os.Exit(cli.RelayMain(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }
