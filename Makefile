.PHONY: build install-bin run-daemon setup test clean

BINARY := clawstore
INSTALL_PATH := /usr/local/bin/$(BINARY)

# Required: mattn/go-sqlite3 (used for sqlite-vec/vec0 support) does not
# include FTS5 by default. This build tag enables it. Without it, the binary
# builds but crashes at startup with "no such module: fts5".
# First-time setup on any machine: run `go env -w GOFLAGS="-tags=fts5"`
# OR just use the Makefile targets below — they pass the tag automatically.
BUILDFLAGS := -tags=fts5

build:
	go build $(BUILDFLAGS) -o $(BINARY) .

install-bin: build
	cp $(BINARY) $(INSTALL_PATH)
	@echo "Installed to $(INSTALL_PATH)"

run-daemon: build
	./$(BINARY) serve --port 7433

setup: install-bin
	$(BINARY) install
	@echo "Daemon installed and started"
	@echo "Pull embedding model: ollama pull nomic-embed-text"

test:
	go test $(BUILDFLAGS) ./...

clean:
	rm -f $(BINARY)
