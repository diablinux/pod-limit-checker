.PHONY: help build test clean fmt vet lint docker push

# Default target
help:
	@echo "Available targets:"
	@echo "  build     - Build the binary"
	@echo "  test      - Run tests"
	@echo "  clean     - Remove build artifacts"
	@echo "  fmt       - Format Go code"
	@echo "  vet       - Run go vet"
	@echo "  lint      - Run golangci-lint"
	@echo "  docker    - Build Docker image"
	@echo "  help      - Show this help"

# Build the binary
build:
	go build -o pod-limit-checker .

# Run tests
test:
	go test ./... -v

# Clean build artifacts
clean:
	rm -f pod-limit-checker
	rm -f pod-limit-checker.exe
	rm -rf dist/
	rm -rf coverage.out

# Format code
fmt:
	go fmt ./...

# Run go vet
vet:
	go vet ./...

# Run golangci-lint (if installed)
lint:
	golangci-lint run

# Build Docker image
docker:
	docker build -t pod-limit-checker:latest .

# Install dependencies
deps:
	go mod download
	go mod tidy

# Run with sample (development)
run:
	go run . --namespace default

# Cross-compile for multiple platforms
cross-build:
	GOOS=linux GOARCH=amd64 go build -o dist/pod-limit-checker-linux-amd64 .
	GOOS=darwin GOARCH=amd64 go build -o dist/pod-limit-checker-darwin-amd64 .
	GOOS=windows GOARCH=amd64 go build -o dist/pod-limit-checker-windows-amd64.exe .

# Generate test coverage
coverage:
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html