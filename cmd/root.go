package cmd

import (
	"fmt"
	"os"

	"cli/api"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var version string


// Entrypoint for the CLI
var rootCmd = &cobra.Command{
	Use:     "podflow",
	Short:   "Endpoint development tool for runpod.io",
	Long:    "The opinionated RunPod tool to develop serverless applications on runpod.io.",
}

func GetRootCmd() *cobra.Command {
	return rootCmd
}

func init() {
	cobra.EnableCommandSorting = false
	cobra.OnInitialize(initConfig)
	registerCommands()
}

func registerCommands() {
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(projectCmd)
	rootCmd.AddCommand(updateCmd)
	//rootCmd.AddCommand(sshCmd)

	// Version
	rootCmd.Version = version
	rootCmd.Flags().BoolP("version", "v", false, "Print the version of podflow")
	rootCmd.SetVersionTemplate(`{{printf "podflow %s\n" .Version}}`)

	// API Access
	rootCmd.PersistentFlags().StringVar(&apiKey, "api-key", "", "RunPod API key")
	rootCmd.PersistentFlags().StringVar(&apiUrl, "api-url", "https://api.runpod.io/graphql", "RunPod API URL")
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute(ver string) {
	version = ver
	api.Version = ver
	rootCmd.Version = ver
	rootCmd.CompletionOptions.HiddenDefaultCmd = true

	rootCmd.SetHelpCommand(&cobra.Command{Use: "no-help", Hidden: true})

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	home, err := os.UserHomeDir()
	cobra.CheckErr(err)
	configPath := home + "/.runpod"

	viper.AddConfigPath(configPath)
	viper.SetConfigType("toml")
	viper.SetConfigName("config.toml")

	viper.SetDefault("apiKey", "")
	viper.SetDefault("apiUrl", "https://api.runpod.io/graphql")

	ConfigFile = configPath + "/config.toml"

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in, otherwise create it.
	if err := viper.ReadInConfig(); err == nil {
		// fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	} else {
		err := os.MkdirAll(configPath, os.ModePerm)
		cobra.CheckErr(err)
		err = viper.WriteConfigAs(ConfigFile)
		cobra.CheckErr(err)
	}

	// Override API access if flags are set
	viper.BindPFlag("apiKey", rootCmd.PersistentFlags().Lookup("api-key"))
	viper.BindPFlag("apiUrl", rootCmd.PersistentFlags().Lookup("api-url"))
}
