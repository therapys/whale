BIN := bin/whale
PKG := ./...

.PHONY: build run tidy lint test clean

build:
	@echo "Building $(BIN)"
	@mkdir -p bin
	@go build -o $(BIN) ./cmd/whale

run: build
	@$(BIN) $(ARGS)

tidy:
	@go mod tidy

lint:
	@go vet $(PKG)

test:
	@echo "No tests yet"
	@true

clean:
	@rm -rf bin


