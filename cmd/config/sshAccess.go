package config

import (
	"cli/api"
	"cli/cmd/ssh"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var GenKeyCmd = &cobra.Command{
	Use:  "ssh-key",
	Short: "Generate an SSH key pair",
	Long: "Generate an SSH key pair for use with RunPod",
	Run: func(c *cobra.Command, args []string) {
		if err := viper.WriteConfig(); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
			return
		}
		fmt.Println("Configuration saved to file:", viper.ConfigFileUsed())

		publicKey, err := ssh.GenerateSSHKeyPair("RunPod-Key-Go")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to generate SSH key: %v\n", err)
			return
		}

		if err := api.AddPublicSSHKey(publicKey); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to add the SSH key: %v\n", err)
			return
		}
		fmt.Println("SSH key added successfully.")
	},
}
