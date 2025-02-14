package config

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var AddKeyCmd = &cobra.Command{
	Use:   "api-key [runpod.io API key]",
	Short: "Set the API key for runpod.io",
	Long:  "Set the API key for runpod.io",
	Args:  cobra.ExactArgs(1),
	Run: func(c *cobra.Command, args []string) {
		fmt.Println("Storing API key...")
		viper.Set("apiKey", args[0])
		if err := viper.WriteConfig(); err != nil {
			fmt.Printf("Error saving config: %v\n", err)
			return
		}
		fmt.Println("API key saved.")
	},
}

var UrlCmd = &cobra.Command{
	Use:  "api-url [runpod.io API URL]",
	Short: "Set alternate API URL for runpod.io",
	Long: "Set alternate API URL for runpod.io (default: https://api.runpod.io/graphql)",
	Args: cobra.ExactArgs(1),
	Run: func(c *cobra.Command, args []string) {
		fmt.Println("Storing API URL...")
		viper.Set("apiUrl", args[0])
		if err := viper.WriteConfig(); err != nil {
			fmt.Printf("Error saving config: %v\n", err)
			return
		}
		fmt.Println("API URL saved.")
	},
}
