package project

import (
	"cli/cmd/nav"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/go-git/go-git/v5"
)

var repositories = []nav.Option{
	{Name: "worker-vllm", Value: "https://github.com/runpod-workers/worker-vllm/tree/cli-integration"},
	{Name: "Provide a GitHub URL", Value: "custom"},
}

func forkProject(gitHubUrl string) {
	fmt.Print("Forking project from GitHub... ")

	// Extract the project name from the GitHub URL
	parts := strings.Split(gitHubUrl, "/")
	projectName := strings.TrimSuffix(parts[len(parts)-1], ".git")

	// Check if the directory already exists
	if _, err := os.Stat(projectName); !os.IsNotExist(err) {
		log.Fatalf("Directory %s already exists. Aborting to prevent overwriting.", projectName)
	}

	// Clone the repository into a temporary directory
	tempDir, err := os.MkdirTemp("", "temp-repo-")
	if err != nil {
		log.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	repo, err := git.PlainClone(tempDir, false, &git.CloneOptions{
		URL:      gitHubUrl,
		Progress: os.Stdout,
	})
	if err != nil {
		log.Fatalf("Failed to clone repository: %v", err)
	}
	fmt.Println("Repository cloned:", repo)

	// Copy files from the temporary directory to the current directory
	err = copyFiles(os.DirFS(tempDir), ".", projectName)
	if err != nil {
		log.Fatalf("Failed to copy files: %v", err)
	}
	fmt.Println("Project forked successfully.")
}
