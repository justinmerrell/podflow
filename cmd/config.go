package cmd

import (
	"cli/cmd/config"

	"github.com/spf13/cobra"
)

var ConfigFile string
var	apiKey     string
var	apiUrl     string


var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage tool configuration",
	Long:  "Configuration and settings for podflow",
}

func init() {
	configCmd.AddCommand(config.AddKeyCmd)
	configCmd.AddCommand(config.UrlCmd)
	configCmd.AddCommand(config.GenKeyCmd)
}
