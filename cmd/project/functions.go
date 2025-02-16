package project

import (
	"cli/api"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/pelletier/go-toml"
)

// TODO: embed all hidden files even those not at top level
//
//go:embed starter_examples/* starter_examples/*/.*
var starterTemplates embed.FS

//go:embed exampleDockerfile
var dockerfileTemplate embed.FS

const basePath string = "starter_examples"

func baseDockerImage(cudaVersion string) string {
	return fmt.Sprintf("runpod/base:0.4.4-cuda%s", cudaVersion)
}

func copyFiles(files fs.FS, source string, dest string) error {
	return fs.WalkDir(files, source, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Skip the base directory
		if path == source {
			return nil
		}

		relPath, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}

		// Generate the corresponding path in the new project folder
		newPath := filepath.Join(dest, relPath)
		if d.IsDir() {
			if err := os.MkdirAll(newPath, os.ModePerm); err != nil {
				return err
			}
		} else {
			content, err := fs.ReadFile(files, path)
			if err != nil {
				return err
			}
			if err := os.WriteFile(newPath, content, 0644); err != nil {
				return err
			}
		}
		return nil
	})
}



func createNewProject(projectName string, cudaVersion string, pythonVersion string, modelType string, modelName string, initCurrentDir bool) {
	projectFolder, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get current working directory: %v", err)
	}

	if !initCurrentDir {
		projectFolder = filepath.Join(projectFolder, projectName)

		if _, err := os.Stat(projectFolder); os.IsNotExist(err) {
			if err := os.Mkdir(projectFolder, 0755); err != nil {
				log.Fatalf("Failed to create project directory: %v", err)
			}
		}

		if modelType == "" {
			modelType = "default"
		}

		if modelName == "" {
			modelName = getDefaultModelName(modelType)
		}

		examplePath := fmt.Sprintf("%s/%s", basePath, modelType)
		err = copyFiles(starterTemplates, examplePath, projectFolder)
		if err := copyFiles(starterTemplates, examplePath, projectFolder); err != nil {
			log.Fatalf("Failed to copy starter example: %v", err)
		}

		// Swap out the model name in handler.py
		handlerPath := fmt.Sprintf("%s/src/handler.py", projectFolder)
		handlerContentBytes, _ := os.ReadFile(handlerPath)
		handlerContent := string(handlerContentBytes)
		handlerContent = strings.ReplaceAll(handlerContent, "<<MODEL_NAME>>", modelName)
		os.WriteFile(handlerPath, []byte(handlerContent), 0644)

		requirementsPath := fmt.Sprintf("%s/builder/requirements.txt", projectFolder)
		requirementsContentBytes, _ := os.ReadFile(requirementsPath)
		requirementsContent := string(requirementsContentBytes)
		//in requirements, replace <<RUNPOD>> with runpod-python import
		//TODO determine version to lock runpod-python at
		requirementsContent = strings.ReplaceAll(requirementsContent, "<<RUNPOD>>", "runpod")
		os.WriteFile(requirementsPath, []byte(requirementsContent), 0644)
	}

	generateProjectToml(projectFolder, "runpod.toml", projectName, cudaVersion, pythonVersion)
}

func loadProjectConfig() *toml.Tree {
	projectFolder, _ := os.Getwd()
	tomlPath := filepath.Join(projectFolder, "runpod.toml")
	toml, err := toml.LoadFile(tomlPath)
	if err != nil {
		panic("runpod.toml not found in the current directory.")
	}
	return toml

}

func getProjectPod(projectId string) (string, error) {
	pods, err := api.GetPods()
	if err != nil {
		return "ERROR", err
	}
	for _, pod := range pods {
		if strings.Contains(pod.Name, projectId) {
			return pod.Id, nil
		}
	}
	return "", errors.New("pod does not exist for project")
}
func getProjectEndpoint(projectId string) (string, error) {
	endpoints, err := api.GetEndpoints()
	if err != nil {
		return "", err
	}
	for _, endpoint := range endpoints {
		if strings.Contains(endpoint.Name, projectId) {
			fmt.Println(endpoint.Id)
			return endpoint.Id, nil
		}
	}
	return "", errors.New("endpoint does not exist for project")
}

func attemptPodLaunch(config *toml.Tree, networkVolumeId string, environmentVariables map[string]string, selectedGpuTypes []string) (pod map[string]interface{}, err error) {
	projectConfig := config.Get("project").(*toml.Tree)
	//attempt to launch a pod with the given configuration.
	for _, gpuType := range selectedGpuTypes {
		fmt.Printf("Trying to get a Pod with %s... ", gpuType)
		podEnv := mapToApiEnv(environmentVariables)
		input := api.CreatePodInput{
			CloudType:         "ALL",
			ContainerDiskInGb: int(projectConfig.Get("container_disk_size_gb").(int64)),
			// DeployCost:      projectConfig.Get(""),
			DockerArgs:      "",
			Env:             podEnv,
			GpuCount:        int(projectConfig.Get("gpu_count").(int64)),
			GpuTypeId:       gpuType,
			ImageName:       projectConfig.Get("base_image").(string),
			MinMemoryInGb:   1,
			MinVcpuCount:    1,
			Name:            fmt.Sprintf("%s-dev (%s)", config.Get("name"), projectConfig.Get("uuid")),
			NetworkVolumeId: networkVolumeId,
			Ports:           strings.ReplaceAll(projectConfig.Get("ports").(string), " ", ""),
			SupportPublicIp: true,
			StartSSH:        true,
			// TemplateId:      projectConfig.Get(""),
			VolumeInGb:      0,
			VolumeMountPath: projectConfig.Get("volume_mount_path").(string),
		}
		pod, err := api.CreatePod(&input)
		if err != nil {
			fmt.Println("Unavailable.")
			continue
		}
		fmt.Println("Success!")
		return pod, nil
	}
	return nil, errors.New("none of the selected GPU types were available")
}

func launchDevPod(config *toml.Tree, networkVolumeId string) (string, error) {
	fmt.Println("Deploying project Pod on RunPod...")
	//construct env vars
	environmentVariables := createEnvVars(config)
	// prepare gpu types
	selectedGpuTypes := []string{}
	tomlGpuTypes := config.GetPath([]string{"project", "gpu_types"})
	if tomlGpuTypes != nil {
		for _, v := range tomlGpuTypes.([]interface{}) {
			selectedGpuTypes = append(selectedGpuTypes, v.(string))
		}
	}
	tomlGpu := config.GetPath([]string{"project", "gpu"}) //legacy
	if tomlGpu != nil {
		selectedGpuTypes = append(selectedGpuTypes, tomlGpu.(string))
	}
	// attempt to launch a pod with the given configuration
	new_pod, err := attemptPodLaunch(config, networkVolumeId, environmentVariables, selectedGpuTypes)
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	fmt.Printf("Check on Pod status at https://www.runpod.io/console/pods/%s\n", new_pod["id"].(string))
	return new_pod["id"].(string), nil
}

func createEnvVars(config *toml.Tree) map[string]string {
	environmentVariables := map[string]string{}
	tomlEnvVars := config.GetPath([]string{"project", "env_vars"})
	if tomlEnvVars != nil {
		tomlEnvVarsMap := tomlEnvVars.(*toml.Tree).ToMap()
		for k, v := range tomlEnvVarsMap {
			environmentVariables[k] = v.(string)
		}
	}
	environmentVariables["RUNPOD_PROJECT_ID"] = config.GetPath([]string{"project", "uuid"}).(string)
	return environmentVariables
}

func mapToApiEnv(env map[string]string) []*api.PodEnv {
	podEnv := []*api.PodEnv{}
	for k, v := range env {
		podEnv = append(podEnv, &api.PodEnv{Key: k, Value: v})
	}
	return podEnv
}

func formatAsDockerEnv(env map[string]string) string {
	result := ""
	for k, v := range env {
		result += fmt.Sprintf("ENV %s=%s\n", k, v)
	}
	return result
}

func startProject(networkVolumeId string) error {
	//parse project toml
	config := loadProjectConfig()
	fmt.Println(config)

	// Project ID
	projectId, ok := config.GetPath([]string{"project", "uuid"}).(string)
	if !ok {
		return fmt.Errorf("project ID not found in config")
	}

	// Project Name
	projectName, ok := config.GetPath([]string{"name"}).(string)
	if !ok {
		return fmt.Errorf("project name not found in config")
	}

	//check for existing pod
	projectPodId, err := getProjectPod(projectId)
	if projectPodId == "ERROR" && err != nil {
		fmt.Println("Error getting project pod: ", err)
		return err
	}

	if projectPodId == "" {
		//or try to get pod with one of gpu types
		projectPodId, err = launchDevPod(config, networkVolumeId)
		if err != nil {
			return err
		}
	}

	//open ssh connection
	sshConn, err := PodSSHConnection(projectPodId)
	if err != nil {
		fmt.Println("error establishing SSH connection to Pod: ", err)
		return err
	}

	fmt.Println(fmt.Sprintf("Project %s Pod (%s) created.", projectName, projectPodId))
	//create remote folder structure
	projectConfig := config.Get("project").(*toml.Tree)
	volumePath := projectConfig.Get("volume_mount_path").(string)
	projectPathUuid := path.Join(volumePath, projectConfig.Get("uuid").(string))
	projectPathUuidDev := path.Join(projectPathUuid, "dev")
	projectPathUuidProd := path.Join(projectPathUuid, "prod")
	remoteProjectPath := path.Join(projectPathUuidDev, projectName)
	var fastAPIPort int
	if strings.Contains(projectConfig.Get("ports").(string), "8080/http") && !strings.Contains(projectConfig.Get("ports").(string), "7270/http") {
		fastAPIPort = 8080
	} else {
		fastAPIPort = 7270
	}
	fmt.Printf("Checking remote project folder: %s on Pod %s\n", remoteProjectPath, projectPodId)
	sshConn.RunCommands([]string{fmt.Sprintf("mkdir -p %s %s", remoteProjectPath, projectPathUuidProd)})
	//rsync project files
	fmt.Printf("Syncing files to Pod %s\n", projectPodId)
	cwd, _ := os.Getwd()
	sshConn.Rsync(cwd, projectPathUuidDev, false)
	//activate venv on remote
	venvPath := "/" + path.Join(projectId, "venv")
	archivedVenvPath := path.Join(projectPathUuid, "dev-venv.tar.zst")
	fmt.Printf("Activating Python virtual environment %s on Pod %s\n", venvPath, projectPodId)
	sshConn.RunCommands([]string{
		fmt.Sprint(`
		DEPENDENCIES=("wget" "sudo" "lsof" "git" "rsync" "zstd")

		function check_and_install_dependencies() {
			for dep in "${DEPENDENCIES[@]}"; do
				if ! command -v $dep &> /dev/null; then
					echo "$dep could not be found, attempting to install..."
					apt-get update && apt-get install -y $dep
					if [ $? -eq 0 ]; then
						echo "$dep installed successfully."
					else
						echo "Failed to install $dep."
						exit 1
					fi
				fi
			done

			# Specifically check for inotifywait command from inotify-tools package
			if ! command -v inotifywait &> /dev/null; then
				echo "inotifywait could not be found, attempting to install inotify-tools..."
				if apt-get install -y inotify-tools; then
					echo "inotify-tools installed successfully."
				else
					echo "Failed to install inotify-tools."
					exit 1
				fi
			fi

			# Specifically check for uv using which command
			if ! command -v uv &> /dev/null; then
				echo "uv could not be found, attempting to install uv..."
				if pip install uv; then
					echo "uv installed successfully."
				else
					echo "Failed to install uv."
					exit 1
				fi
			fi

			wget -qO- cli.runpod.net | sudo bash &> /dev/null
		}
		check_and_install_dependencies`),
		fmt.Sprintf(`
		if ! [ -f %s/bin/activate ]
		then
			if [ -f %s ]
			then
				echo "Retrieving existing venv from network volume..."
				mkdir -p %s && tar -xf %s -C %s
			else
				echo "Creating new venv..."
				uv venv --python=%s %s
			fi
		fi`, venvPath, archivedVenvPath, venvPath, archivedVenvPath, venvPath, config.GetPath([]string{"runtime", "python_version"}).(string), venvPath),
		fmt.Sprintf(`source %s/bin/activate &&
		cd %s &&
		uv pip install -v --requirement %s`,

			// python -m pip install --upgrade pip &&
			// python -m pip install -v --requirement %s --report /installreport.json`,

			venvPath, remoteProjectPath, config.GetPath([]string{"runtime", "requirements_path"}).(string)),
	})

	//create file watcher
	fmt.Println("Creating Project watcher...")
	go sshConn.SyncDir(cwd, projectPathUuidDev)

	//run launch api server / hot reload loop
	pipReqPath := path.Join(remoteProjectPath, config.GetPath([]string{"runtime", "requirements_path"}).(string))
	handlerPath := path.Join(remoteProjectPath, config.GetPath([]string{"runtime", "handler_path"}).(string))
	launchApiServer := fmt.Sprintf(`
		#!/bin/bash
		if [ -z "${BASE_RELEASE_VERSION}" ]; then
			API_PORT=%d
			PRINTED_API_PORT=$API_PORT
		else
			API_PORT=7271
			PRINTED_API_PORT=7270
		fi
		API_HOST="0.0.0.0"
		PYTHON_VENV_PATH="%s" # Path to the Python virutal environment used during development located on the Pod at /<project_id>/venv
		PROJECT_DIRECTORY="%s/%s"
		VENV_ARCHIVE_PATH="%s"
		HANDLER_PATH="%s"
		REQUIRED_FILES="%s"

		pkill inotify # Kill any existing inotify processes

		function start_api_server {
			lsof -ti:$API_PORT | xargs kill -9 2>/dev/null # Kill the old API server if it's still running
			python $1 --rp_serve_api --rp_api_host="$API_HOST" --rp_api_port=$API_PORT --rp_api_concurrency=1 &
			SERVER_PID=$!
		}

		function force_kill {
			if [[ -z "$1" ]]; then
				echo "No PID provided for force_kill."
				return
			fi

			kill $1 2>/dev/null

			for i in {1..5}; do  # Wait up to 5 seconds, checking every second.
				if ! ps -p $1 > /dev/null 2>&1; then
					echo "Process $1 has been gracefully terminated."
					return
				fi
				sleep 1
			done

			echo "Graceful kill failed, attempting SIGKILL..."
			kill -9 $1 2>/dev/null

			for i in {1..5}; do  # Wait up to 5 seconds, checking every second.
				if ! ps -p $1 >/dev/null 2>&1; then
					echo "Process $1 has been killed with SIGKILL."
					return
				fi
				sleep 1
			done

			echo "Failed to kill process with PID: $1 after SIGKILL attempt."
    		exit 1
		}

		function cleanup {
			echo "Cleaning up..."
			force_kill $SERVER_PID
		}
		trap cleanup EXIT SIGINT

		if source $PYTHON_VENV_PATH/bin/activate; then
			echo -e "- Activated project environment."
		else
			echo "Failed to activate project environment."
			exit 1
		fi

		if cd $PROJECT_DIRECTORY; then
			echo -e "- Changed to project directory."
		else
			echo "Failed to change directory."
			exit 1
		fi

		function tar_venv {
			# if ! [ $(cat /installreport.json | grep "install" | grep -c "\[\]") -eq 1 ]
			if [ true ]
			then
				tar -c -C $PYTHON_VENV_PATH . | zstd -T0 > /venv.tar.zst;
				mv /venv.tar.zst $VENV_ARCHIVE_PATH ;
				echo "Synced venv to network volume"
			fi
		}

		tar_venv &

		# Start the API server in the background, and save the PID
		start_api_server $HANDLER_PATH

		echo -e "- Started API server with PID: $SERVER_PID" && echo ""
		echo "Connect to the API server at:"
		echo ">  https://$RUNPOD_POD_ID-$PRINTED_API_PORT.proxy.runpod.net" && echo ""

		#like inotifywait, but will only report the name of a file if it shouldn't be ignored according to .runpodignore
		#uses git check-ignore to ensure same syntax as gitignore, but git check-ignore expects to be run in a repo
		#so we must set up a git-repo-like file structure in some temp directory
		function notify_nonignored_file {
			local tmp_dir=$(mktemp -d)
			cp .runpodignore "$tmp_dir/.gitignore"
			cd "$tmp_dir" && git init -q  # Setup a temporary git repo to leverage .gitignore

			local project_directory="$PROJECT_DIRECTORY"

			# Listen for file changes.
			inotifywait -q -r -e modify,create,delete --format '%%w%%f' "$project_directory" | while read -r file; do
				# Convert each file path to a relative path and check if it's ignored by git
				local rel_path=$(realpath --relative-to="$project_directory" "$file")
				if ! git check-ignore -q "$rel_path"; then
					echo "$rel_path"
				fi
			done

			cd -  > /dev/null  # Return to the original directory
			rm -rf "$tmp_dir"
		}
		trap '[[ -n $tmp_dir && -d $tmp_dir ]] && rm -rf "$tmp_dir"' EXIT

		monitor_and_restart() {
			while true; do
				if changed_file=$(notify_nonignored_file); then
					echo "Found changes in: $changed_file"
				else
					echo "No changes found."
					exit 1
				fi

				force_kill $SERVER_PID

				# Install new requirements if requirements.txt was changed
				if [[ $changed_file == *"requirements"* ]]; then
					echo "Installing new requirements..."
					# python -m pip install --upgrade pip && python -m pip install -v -r $REQUIRED_FILES --report /installreport.json
					uv pip install -v -r $REQUIRED_FILES
					tar_venv &
				fi

				# Restart the API server in the background, and save the PID
				start_api_server $HANDLER_PATH

				echo "Restarted API server with PID: $SERVER_PID"
			done
		}

		monitor_and_restart
	`, fastAPIPort, venvPath, projectPathUuidDev, projectName, archivedVenvPath, handlerPath, pipReqPath)
	fmt.Println()
	fmt.Println("Starting project endpoint...")
	sshConn.RunCommand(launchApiServer)
	return nil
}

func deployProject(networkVolumeId string) (endpointId string, err error) {
	//parse project toml
	config := loadProjectConfig()
	projectId := config.GetPath([]string{"project", "uuid"}).(string)
	projectConfig := config.Get("project").(*toml.Tree)
	projectName := config.Get("name").(string)
	projectPathUuid := path.Join(projectConfig.Get("volume_mount_path").(string), projectConfig.Get("uuid").(string))
	projectPathUuidProd := path.Join(projectPathUuid, "prod")
	remoteProjectPath := path.Join(projectPathUuidProd, config.Get("name").(string))
	venvPath := path.Join(projectPathUuidProd, "venv")
	//check for existing pod
	fmt.Println("Finding a pod for initial file sync")
	projectPodId, err := getProjectPod(projectId)
	if projectPodId == "" || err != nil {
		//or try to get pod with one of gpu types
		projectPodId, err = launchDevPod(config, networkVolumeId)
		if err != nil {
			return "", err
		}
	}
	//open ssh connection
	sshConn, err := PodSSHConnection(projectPodId)
	if err != nil {
		fmt.Println("error establishing SSH connection to Pod: ", err)
		return "", err
	}
	//sync remote dev to remote prod
	sshConn.RunCommand(fmt.Sprintf("mkdir -p %s", remoteProjectPath))
	fmt.Printf("Syncing files to Pod %s prod\n", projectPodId)
	cwd, _ := os.Getwd()
	sshConn.Rsync(cwd, projectPathUuidProd, false)
	//activate venv on remote
	fmt.Printf("Activating Python virtual environment: %s on Pod %s\n", venvPath, projectPodId)
	sshConn.RunCommands([]string{
		fmt.Sprintf("python%s -m venv %s", config.GetPath([]string{"runtime", "python_version"}).(string), venvPath),
		fmt.Sprintf(`source %s/bin/activate &&
		cd %s &&
		python -m pip install --upgrade pip &&
		python -m pip install -v --requirement %s`,
			venvPath, remoteProjectPath, config.GetPath([]string{"runtime", "requirements_path"}).(string)),
	})
	env := mapToApiEnv(createEnvVars(config))
	// Construct the docker start command
	handlerPath := path.Join(remoteProjectPath, config.GetPath([]string{"runtime", "handler_path"}).(string))
	activateCmd := fmt.Sprintf(". %s/bin/activate", venvPath)
	pythonCmd := fmt.Sprintf("python -u %s", handlerPath)
	dockerStartCmd := "bash -c \"" + activateCmd + " && " + pythonCmd + "\""
	//deploy new template
	projectEndpointTemplateId, err := api.CreateTemplate(&api.CreateTemplateInput{
		Name:              fmt.Sprintf("%s-endpoint-%s-%d", projectName, projectId, time.Now().UnixMilli()),
		ImageName:         projectConfig.Get("base_image").(string),
		Env:               env,
		DockerStartCmd:    dockerStartCmd,
		IsServerless:      true,
		ContainerDiskInGb: int(projectConfig.Get("container_disk_size_gb").(int64)),
		VolumeMountPath:   projectConfig.Get("volume_mount_path").(string),
		StartSSH:          true,
		IsPublic:          false,
		Readme:            "",
	})
	if err != nil {
		fmt.Println("error making template")
		return "", err
	}
	//deploy / update endpoint
	deployedEndpointId, err := getProjectEndpoint(projectId)
	//default endpoint settings
	minWorkers := 0
	maxWorkers := 3
	flashboot := true
	flashbootSuffix := " -fb"
	idleTimeout := 5
	endpointConfig, ok := config.Get("endpoint").(*toml.Tree)
	if ok {
		if min, ok := endpointConfig.Get("active_workers").(int64); ok {
			minWorkers = int(min)
		}
		if max, ok := endpointConfig.Get("max_workers").(int64); ok {
			maxWorkers = int(max)
		}
		if fb, ok := endpointConfig.Get("flashboot").(bool); ok {
			flashboot = fb
		}
		if !flashboot {
			flashbootSuffix = ""
		}
		if idle, ok := endpointConfig.Get("idle_timeout").(int64); ok {
			idleTimeout = int(idle)
		}
	}
	if err != nil {
		deployedEndpointId, err = api.CreateEndpoint(&api.CreateEndpointInput{
			Name:            fmt.Sprintf("%s-endpoint-%s%s", projectName, projectId, flashbootSuffix),
			TemplateId:      projectEndpointTemplateId,
			NetworkVolumeId: networkVolumeId,
			GpuIds:          "AMPERE_16",
			IdleTimeout:     idleTimeout,
			ScalerType:      "QUEUE_DELAY",
			ScalerValue:     4,
			WorkersMin:      minWorkers,
			WorkersMax:      maxWorkers,
		})
		if err != nil {
			fmt.Println("error making endpoint")
			return "", err
		}
	} else {
		err = api.UpdateEndpointTemplate(deployedEndpointId, projectEndpointTemplateId)
		if err != nil {
			fmt.Println("error updating endpoint template")
			return "", err
		}
	}
	return deployedEndpointId, nil
}

func buildProjectDockerfile() {
	//parse project toml
	config := loadProjectConfig()
	projectConfig := config.Get("project").(*toml.Tree)
	runtimeConfig := config.Get("runtime").(*toml.Tree)
	//build Dockerfile
	dockerfileBytes, _ := dockerfileTemplate.ReadFile("exampleDockerfile")
	dockerfile := string(dockerfileBytes)
	//base image: from toml
	dockerfile = strings.ReplaceAll(dockerfile, "<<BASE_IMAGE>>", projectConfig.Get("base_image").(string))
	//pip requirements
	dockerfile = strings.ReplaceAll(dockerfile, "<<REQUIREMENTS_PATH>>", runtimeConfig.Get("requirements_path").(string))
	dockerfile = strings.ReplaceAll(dockerfile, "<<PYTHON_VERSION>>", runtimeConfig.Get("python_version").(string))
	//cmd: start handler
	dockerfile = strings.ReplaceAll(dockerfile, "<<HANDLER_PATH>>", runtimeConfig.Get("handler_path").(string))
	if includeEnvInDockerfile {
		dockerEnv := formatAsDockerEnv(createEnvVars(config))
		dockerfile = strings.ReplaceAll(dockerfile, "<<SET_ENV_VARS>>", "\n"+dockerEnv)
	} else {
		dockerfile = strings.ReplaceAll(dockerfile, "<<SET_ENV_VARS>>", "")
	}
	//save to Dockerfile in project directory
	projectFolder, _ := os.Getwd()
	dockerfilePath := filepath.Join(projectFolder, "Dockerfile")
	os.WriteFile(dockerfilePath, []byte(dockerfile), 0644)
	fmt.Printf("Dockerfile created at %s\n", dockerfilePath)

}
