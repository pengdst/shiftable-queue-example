.PHONY: help tidy build test mock generate clean run

# Default target
help:
	@echo "Available targets:"
	@echo "  help      - Show this help message"
	@echo "  tidy      - Format and tidy Go modules"
	@echo "  build     - Build the application"
	@echo "  test      - Run all tests with coverage"
	@echo "  mock      - Generate mocks using mockery"
	@echo "  generate  - Alias for mock generation"
	@echo "  clean     - Clean build artifacts and generated files"
	@echo "  run       - Run the application"

# Format and tidy modules
tidy:
	go fmt ./...
	go mod tidy
	go vet ./...

# Build the application
build:
	go build -o bin/shiftable-queue .

# Run tests with coverage
test:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Generate mocks using mockery
mock:
	@echo "Generating mocks..."
	mockery
	@echo "Mocks generated successfully"

# Alias for mock generation
generate: mock

# Clean build artifacts and generated files
clean:
	rm -rf bin/
	rm -rf mocks/
	rm -f mock_*.go
	rm -f coverage.out coverage.html

# Run the application
run:
	go run . -cmd=$(if $(cmd),$(cmd),server)

# Install dependencies
deps:
	go mod download
	go mod verify

# Run tests in watch mode (requires entr)
test-watch:
	find . -name "*.go" | entr -c go test -v ./...

# Run application with live reload (requires air)
run-live:
	air