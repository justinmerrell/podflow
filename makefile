.PHONY: proto

local:
	go build -ldflags "-X 'main.Version=1.0.0'" -o bin/podflow .

linux:
	env GOOS=linux GOARCH=amd64 go build -ldflags "-X 'main.Version=1.0.0'" -o bin/podflow .
mac:
	env GOOS=darwin GOARCH=arm64 go build -ldflags "-X 'main.Version=1.0.0'" -o bin/podflow .
windows:
	env GOOS=windows GOARCH=amd64 go build -ldflags "-X 'main.Version=1.0.0'" -o bin/podflow.exe .
mac-amd64:
	env GOOS=darwin GOARCH=amd64 go build -ldflags "-X 'main.Version=1.0.0'" -o bin/podflow .


lint:
	golangci-lint run
