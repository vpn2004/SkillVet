BIN_DIR := bin
BINARY := $(BIN_DIR)/safespace-rater

.PHONY: build test tidy clean

build:
	@mkdir -p $(BIN_DIR)
	go build -o $(BINARY) ./cmd/safespace-rater

test:
	go test ./...

tidy:
	go mod tidy

clean:
	rm -rf $(BIN_DIR)
