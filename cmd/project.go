package cmd

import (
	"cli/cmd/project"

	"github.com/spf13/cobra"
)

var projectCmd = &cobra.Command{
	Use:   "airfoil [command]",
	Short: "Dockerless solution to Develop and deploy serverless endpoints",
	Long:  "Dockerless solution to develop and deploy serverless projects entirely on RunPod's infrastructure.",
}

func init() {
	projectCmd.AddCommand(project.NewProjectCmd)
	projectCmd.AddCommand(project.StartProjectCmd)
	projectCmd.AddCommand(project.DeployProjectCmd)
	projectCmd.AddCommand(project.BuildProjectCmd)
}
