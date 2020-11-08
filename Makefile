run:
	go run main.go

build:
	go build -o bin/lambda main.go

compile:
	echo "Compiling for every OS and Platform"
	# Linux
	GOOS=linux GOARCH=amd64 go build -o bin/lambda-linux-amd64 main.go
	# Windows binary
	GOOS=windows GOARCH=amd64 go build -o bin/lambda-windows-amd64 main.go