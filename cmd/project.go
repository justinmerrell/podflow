package cmd

import (
	"cli/cmd/project"

	"github.com/spf13/cobra"
)

var projectCmd = &cobra.Command{
	Use:   "project [command]",
	Short: "Manage RunPod projects",
	Long:  "Develop and deploy projects entirely on RunPod's infrastructure.",
}

func init() {
	projectCmd.AddCommand(project.ForkProjectCmd)
	projectCmd.AddCommand(project.NewProjectCmd)
	projectCmd.AddCommand(project.StartProjectCmd)
	projectCmd.AddCommand(project.DeployProjectCmd)
	projectCmd.AddCommand(project.PublishProjectCmd)
	//projectCmd.AddCommand(project.BuildProjectCmd)
}
