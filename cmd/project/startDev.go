package project

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/pelletier/go-toml"
)

func loadAndValidateConfig() (*toml.Tree, error) {
	// Read in runpod.toml from within the current directory.

	//parse project toml
	config := loadProjectConfig()
	if config == nil {
		return nil, fmt.Errorf("failed to load project config")
	}

	// project ID
	projectID, ok := config.GetPath([]string{"project", "uuid"}).(string)
	if !ok || projectID == "" {
		return nil, fmt.Errorf("project ID not found in config")
	}

	// project name
	projectName, ok := config.GetPath([]string{"name"}).(string)
	if !ok || projectName == "" {
		return nil, fmt.Errorf("project name not found in config")
	}

	return config, nil
}


func pickAPIPort(config *toml.Tree) int {
	ports := config.GetPath([]string{"project", "ports"}).(string)
	if strings.Contains(ports, "8080/http") && !strings.Contains(ports, "7270/http") {
		return 8080
	}
	return 7270
}

func ensureProjectPod(config *toml.Tree, networkVolumeId string) (string, error) {
	projectID := config.GetPath([]string{"project", "uuid"}).(string)

	// Attempt to get an existing pod
	projectPodID, err := getProjectPod(projectID)
	if projectPodID=="ERROR" && err != nil {
		// Log + return error if an actual failure
		fmt.Println("Error getting project pod:", err)
		return "", err
	}

	// If no existing pod, launch a new one
	if projectPodID == "" {
		projectPodID, err = launchDevPod(config, networkVolumeId)
		if err != nil {
			return "", fmt.Errorf("failed to launch dev pod: %w", err)
		}
	}
	return projectPodID, nil
}


func setupRemoteEnv(config *toml.Tree, projectPodID string, networkVolumeId string) (*SSHConnection, error) {
	projectName := config.GetPath([]string{"name"}).(string)
	projectConfig := config.Get("project").(*toml.Tree)

	// 1) SSH connection
	sshConn, err := PodSSHConnection(projectPodID)
	if err != nil {
		return nil, fmt.Errorf("failed to establish SSH: %w", err)
	}

	volumePath := projectConfig.Get("volume_mount_path").(string)
	projectPath := path.Join(volumePath, projectConfig.Get("uuid").(string))
	projectPathDev := path.Join(projectPath, "dev")
	projectPathProd := path.Join(projectPath, "prod")
	remoteProjectPath := path.Join(projectPathDev, projectName)

	fmt.Printf("Creating dev/prod directories on remote Pod: %s\n", projectPodID)
	sshConn.RunCommands([]string{
		fmt.Sprintf("mkdir -p %s %s", remoteProjectPath, projectPathProd),
	})

	// 2) Rsync local files -> remote
	cwd, _ := os.Getwd()
	fmt.Printf("Syncing local files from '%s' to '%s' on Pod '%s'\n", cwd, projectPathDev, projectPodID)
	sshConn.Rsync(cwd, projectPathDev, false)

	// 3) Install dependencies (apt + pip) & ensure Python venv
	if err := ensureDependencies(sshConn, config, remoteProjectPath); err != nil {
		return nil, fmt.Errorf("failed to ensure dependencies: %w", err)
	}

	return sshConn, nil
}


func ensureDependencies(sshConn *SSHConnection, config *toml.Tree, remoteProjectPath string) error {
	// Ensure required dependencies for the development workflow are installed on the Pod

	// Project ID, specified in the project config
	projectID := config.GetPath([]string{"project", "uuid"}).(string)

	// Python package manager to use, specified in the project config
	packageManager, ok := config.GetPath([]string{"runtime", "package_manager"}).(string)
	if !ok || packageManager == "" {
		packageManager = "uv" // Set default package manager to "uv"
	}

	// Python version to use, specified in the project config
	pythonVersion := config.GetPath([]string{"runtime", "python_version"}).(string)

	// Path to the requirements file for the project, relative to the project root
	requirementsPath := config.GetPath([]string{"runtime", "requirements_path"}).(string)

	// Path to archived venv on the network volume (default: /runpod-volume/<project_id>/dev-venv.tar.zst)
	archivedVenvPath := path.Join(
		config.GetPath([]string{"project", "volume_mount_path"}).(string),
		projectID,
		"dev-venv.tar.zst",
	)

	// Path to the Python virtual environment used in development and production (default: /<project_id>/venv)
	venvPath := "/" + path.Join(projectID, "venv")

	installScript := fmt.Sprintf(`
		#!/bin/bash
		set -euo pipefail
		IFS=$'\n\t'

		# Error handler to capture the failing command and its line number.
		error_handler() {
			local exit_code=$?
			local line_number=${BASH_LINENO[0]}
			echo "Error: Command '${BASH_COMMAND}' exited with code ${exit_code} at line ${line_number}." >&2
			exit "${exit_code}"
		}
		trap 'error_handler' ERR

		PACKAGE_MANAGER="%s"		# Specified in the project config
		PYTHON_VERSION="%s"	   		# Specified in the project config
		PYTHON_VENV_PATH="%s"		# Path to the active Python virtual environment on the Pod
		ARCHIVED_VENV_PATH="%s" 	# Path to the archived Python virtual environment on the network volume
		REMOTE_PROJECT_PATH="%s"	# Path to the remote project files, typically on the network volume
		REQUIREMENTS_PATH="%s"		# Path to the requirements file for the project, relative to the project root
		DEPENDENCIES=("wget" "sudo" "lsof" "git" "rsync" "zstd")


		function check_and_install_dependencies() {
			apt-get update -qq
			for dep in "${DEPENDENCIES[@]}"; do
				if ! command -v "$dep" &> /dev/null; then
					echo "[INFO] Installing $dep ..."
					apt-get install -y "$dep"
				fi
			done

			if ! command -v inotifywait &> /dev/null; then
				echo "[INFO] Installing inotify-tools ..."
				apt-get install -y inotify-tools
			fi

			if ! command -v uv &> /dev/null; then
				echo "[INFO] Installing uv ..."
				pip install uv
			fi

			# runpod CLI
			wget -qO- cli.runpod.net | sudo bash &> /dev/null
		}

		function create_or_extract_venv() {
			if [ ! -f "$PYTHON_VENV_PATH/bin/activate" ]; then
				if [ -f "$ARCHIVED_VENV_PATH" ]; then
					echo "[INFO] Extracting existing venv from archive: $ARCHIVED_VENV_PATH"
					mkdir -p "$REMOTE_PROJECT_PATH"
					tar --use-compress-program="zstd -d --threads=0" -xf "$ARCHIVED_VENV_PATH" -C "$REMOTE_PROJECT_PATH"
				else
					echo "[INFO] Creating new venv with $PACKAGE_MANAGER..."
					if [ "$PACKAGE_MANAGER" == "pip" ]; then
						python"$PYTHON_VERSION" -m venv --upgrade-deps "$PYTHON_VENV_PATH"
					elif [ "$PACKAGE_MANAGER" == "uv" ]; then
						uv venv --python=python"$PYTHON_VERSION" "$PYTHON_VENV_PATH"
					else
						echo "[ERROR] Unsupported package manager: $PACKAGE_MANAGER"
						exit 1
					fi
				fi
			fi
		}

		function install_python_packages() {
			if source "$PYTHON_VENV_PATH/bin/activate"; then
				echo "[INFO] Activated venv at $PYTHON_VENV_PATH"
				cd "$REMOTE_PROJECT_PATH"
				echo "Current directory: $(pwd)"

				# Install Python dependencies from requirements.txt
				if [ "$PACKAGE_MANAGER" == "pip" ]; then
					pip install -v --requirement "$REQUIREMENTS_PATH" --report /installreport.json hf_transfer
				elif [ "$PACKAGE_MANAGER" == "uv" ]; then
					uv pip install --requirement "$REQUIREMENTS_PATH" hf_transfer
				else
					echo "[ERROR] Unsupported package manager: $PACKAGE_MANAGER"
				fi

				echo "[INFO] Installed Python dependencies."
			else
				echo "[ERROR] Failed to activate venv."
				exit 1
			fi
		}

		# 1) Install apt & pip dependencies
		check_and_install_dependencies

		# 2) Create or extract virtual environment
		create_or_extract_venv

		# 3) Install Python packages
		install_python_packages
		`, packageManager, pythonVersion, venvPath, archivedVenvPath, remoteProjectPath, requirementsPath,
	)

	if err := sshConn.RunCommand(installScript); err != nil {
		return fmt.Errorf("dependency installation script failed: %w", err)
	}

	return nil
}


func launchAPIServer(sshConn *SSHConnection, config *toml.Tree, projectName, projectPodID, localDir, remoteProjectPath string) error {
	projectID := config.GetPath([]string{"project", "uuid"}).(string)

	// Python package manager to use, specified in the project config
	packageManager, ok := config.GetPath([]string{"runtime", "package_manager"}).(string)
	if !ok || packageManager == "" {
		packageManager = "uv" // Set default package manager to "uv"
	}

	runtimeTree := config.Get("runtime").(*toml.Tree)
	requirementsPath := path.Join(remoteProjectPath, runtimeTree.Get("requirements_path").(string))
	handlerPath := path.Join(remoteProjectPath, runtimeTree.Get("handler_path").(string))

	// Decide which port to use
	fastAPIPort := pickAPIPort(config)

	venvPath := "/" + path.Join(projectID, "venv")
	archivedVenvPath := path.Join(
		config.GetPath([]string{"project", "volume_mount_path"}).(string),
		projectID,
		"dev-venv.tar.zst",
	)

	serverScript := fmt.Sprintf(`
		#!/bin/bash
		set -euo pipefail
		IFS=$'\n\t'

		# Error handler to capture the failing command and its line number.
		error_handler() {
			local exit_code=$?
			local line_number=${BASH_LINENO[0]}
			echo "Error: Command '${BASH_COMMAND}' exited with code ${exit_code} at line ${line_number}." >&2
			exit "${exit_code}"
		}
		trap 'error_handler' ERR

		API_PORT=%d
		API_HOST="0.0.0.0"
		PACKAGE_MANAGER="%s"		# Specified in the project config
		PYTHON_VENV_PATH="%s" 		# Path to the Python virtual environment used during development located on the Pod at /<project_id>/venv
		ARCHIVED_VENV_PATH="%s"
		REQUIRED_FILES="%s"
		HANDLER_PATH="%s"
		PROJECT_DIRECTORY="%s"

		if [ -z "${BASE_RELEASE_VERSION}" ]; then
			PRINTED_API_PORT=$API_PORT
		else
			API_PORT=7271
			PRINTED_API_PORT=7270
		fi

		# Change to the project directory
		if cd "$PROJECT_DIRECTORY"; then
			echo -e "- Changed to project directory."
		else
			echo "Failed to change directory."
			exit 1
		fi

		# --- Functions ---

		function start_api_server {
			# Kill any process listening on API_PORT
    		lsof -ti:"$API_PORT" | xargs --no-run-if-empty kill -9 2>/dev/null || true
			# Start the API server in the background
			python $1 --rp_serve_api --rp_api_host="$API_HOST" --rp_api_port=$API_PORT --rp_api_concurrency=1 &
			SERVER_PID=$!
		}


		wait_for_pid() {
			local pid="$1"
			local timeout="$2"
			for ((i = 0; i < timeout; i++)); do
				# kill -0 doesn't send a signal but checks for the existence of the process.
				if ! kill -0 "$pid" 2>/dev/null; then
					return 0  # Process is gone.
				fi
				sleep 1
			done
			return 1  # Timed out waiting for the process to disappear.
		}

		force_kill() {
			local pid="$1"

			if [[ -z "$pid" ]]; then
				echo "No PID provided for force_kill." >&2
				return 1
			fi

			# Attempt graceful termination (SIGTERM)
			kill "$pid" 2>/dev/null

			if wait_for_pid "$pid" 5; then
				echo "Process $pid has been gracefully terminated."
				return 0
			fi

			echo "Graceful kill failed, attempting SIGKILL..."
			kill -9 "$pid" 2>/dev/null

			if wait_for_pid "$pid" 5; then
				echo "Process $pid has been killed with SIGKILL."
				return 0
			fi

			echo "Failed to kill process with PID: $pid after SIGKILL attempt." >&2
			return 1
		}


		function tar_venv {
			# Archive the virtual environment and move it to the network volume.
    		tar -c -C "$PYTHON_VENV_PATH" . | zstd -T0 > /venv.tar.zst
    		mv /venv.tar.zst "$ARCHIVED_VENV_PATH"
    		echo "Synced venv to network volume"
		}

		# Run tar_venv in the background initially.
		tar_venv &


		function cleanup {
			echo "Cleaning up..."
			force_kill $SERVER_PID
		}
		trap cleanup EXIT SIGINT


		#like inotifywait, but will only report the name of a file if it shouldn't be ignored according to .runpodignore
		#uses git check-ignore to ensure same syntax as gitignore, but git check-ignore expects to be run in a repo
		#so we must set up a git-repo-like file structure in some temp directory
		function notify_nonignored_file {
			local tmp_dir project_directory file rel_path

			# Create a temporary directory and ensure cleanup on exit.
			tmp_dir=$(mktemp -d) || { echo "Failed to create temp directory" >&2; return 1; }

			(
				trap 'rm -rf "$tmp_dir"' EXIT

				# Copy .runpodignore as .gitignore and initialize a temporary git repo to leverage .gitignore
				cp .runpodignore "$tmp_dir/.gitignore" || { echo "Failed to copy .runpodignore" >&2; return 1; }
				git --git-dir="$tmp_dir" init -q || { echo "Failed to initialize temporary git repo" >&2; return 1; }
				project_directory="${PROJECT_DIRECTORY:-.}"

				# Listen for file changes and output non-ignored files.
				inotifywait -q -r -e modify,create,delete --format '%%w%%f' "$project_directory" | while read -r file; do

					# Convert the absolute file path to one relative to the project.
					rel_path=$(realpath --relative-to="$project_directory" "$file")

					# If the file is not ignored according to the temporary .gitignore, output its name.
					if ! git --git-dir="$tmp_dir" --work-tree="$project_directory" check-ignore -q "$rel_path"; then
						echo "$rel_path"
						exit 0 # Exit after the first non-ignored file is found.
					fi
				done
			)
		}


		monitor_and_restart() {
			while true; do
				local changed_file

				# Wait for a non-ignored file change.
				if ! changed_file=$(notify_nonignored_file); then
					echo "No changes found." >&2
					return 1
				fi

				echo "Found changes in: $changed_file"

				# Kill the current server.
				force_kill "$SERVER_PID"

				# If any file related to requirements was changed, update them.
				if [[ $changed_file == *"requirements"* ]]; then
					echo "Installing new requirements..."
					if [ "$PACKAGE_MANAGER" == "pip" ]; then
						python -m pip install --upgrade pip && python -m pip install -v --requirement $REQUIRED_FILES --report /installreport.json hf_transfer
					elif [ "$PACKAGE_MANAGER" == "uv" ]; then
						uv pip install --requirement $REQUIREMENTS_PATH hf_transfer
					else
						echo "[ERROR] Unsupported package manager: $PACKAGE_MANAGER"
					fi

					# Tar the venv and sync it to the network volume in the background
					tar_venv &
				fi

				# Restart the API server in the background, and save the PID
				start_api_server $HANDLER_PATH
				echo "Restarted API server with PID: $SERVER_PID"
			done
		}

		# --- Main Execution ---

		if source $PYTHON_VENV_PATH/bin/activate; then
			echo -e "- Activated project environment."
		else
			echo "Failed to activate project environment." >&2
			exit 1
		fi

		start_api_server $HANDLER_PATH
		echo -e "- Started API server with PID: $SERVER_PID"
		echo ""
		echo "Connect to the API server at:"
		echo ">  https://$RUNPOD_POD_ID-$PRINTED_API_PORT.proxy.runpod.net"
		echo ""

		monitor_and_restart
		`, fastAPIPort, packageManager, venvPath, archivedVenvPath, requirementsPath, handlerPath, remoteProjectPath)

	// Actually run it
	fmt.Println("Launching API server with hot reload on Pod:", projectPodID)
	sshConn.RunCommand(serverScript)
	return nil
}

func startProject(networkVolumeId string) error {
	// 1) Load + validate config
	config, err := loadAndValidateConfig()
	if err != nil {
		return fmt.Errorf("failed to load/validate config: %w", err)
	}
	fmt.Println("Loaded project config.")

	// 2) Ensure we have a Pod (reuse or create)
	podID, err := ensureProjectPod(config, networkVolumeId)
	if err != nil {
		return fmt.Errorf("failed to ensure pod: %w", err)
	}
	projectName := config.GetPath([]string{"name"}).(string)
	fmt.Printf("Pod ready. Project '%s' Pod ID: %s\n", projectName, podID)

	// 3) Setup remote environment (SSH, directories, dependencies)
	sshConn, err := setupRemoteEnv(config, podID, networkVolumeId)
	if err != nil {
		return fmt.Errorf("setupRemoteEnv failed: %w", err)
	}

	// 4) Start file watcher in background
	projectConfig := config.Get("project").(*toml.Tree)
	volumePath := projectConfig.Get("volume_mount_path").(string)
	projectPath := path.Join(volumePath, projectConfig.Get("uuid").(string), "dev")
	cwd, _ := os.Getwd()

	fmt.Println("Starting file watcher for hot reload...")
	go sshConn.SyncDir(cwd, projectPath)

	// 5) Launch the API server with hot reload
	err = launchAPIServer(sshConn, config, projectName, podID, cwd, path.Join(projectPath, projectName))
	if err != nil {
		return fmt.Errorf("failed to launch API server: %w", err)
	}

	return nil
}
