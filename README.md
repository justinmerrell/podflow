<div align="center">

# PodFlow Development Tool

podflow is the CLI tool to automate / manage GPU pods for [runpod.io](https://runpod.io).

_Note: All pods automatically come with runpodctl installed with a pod-scoped API key._

</div>

## Table of Contents

- [PodFlow Development Tool](#podflow-development-tool)
  - [Table of Contents](#table-of-contents)
  - [Get Started](#get-started)
    - [Install](#install)
      - [Linux/MacOS (WSL)](#linuxmacos-wsl)
      - [MacOS](#macos)
      - [Windows PowerShell](#windows-powershell)

## Get Started

### Install

#### Linux/MacOS (WSL)

```bash
# Download and install via wget
wget -qO- cli.runpod.net | sudo bash

wget -qO- https://raw.githubusercontent.com/justinmerrell/podflow/master/install.sh | sudo bash
```

#### MacOS

```bash
# Using homebrew
brew tap runpod/runpodctl
brew install runpodctl
```

#### Windows PowerShell

```powershell
wget https://github.com/justinmerrell/podflow/releases/download/0.0.1/podflow-windows-amd64.exe -O podflow.exe
```

For a comprehensive list of commands, visit [RunPod CLI documentation](doc/runpod.md).
