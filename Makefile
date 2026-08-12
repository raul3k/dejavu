BIN := dejavu
PREFIX ?= $(HOME)/.local

.PHONY: all build test fmt vet install uninstall clean

all: install

build:
	go build -o $(BIN) .

test:
	go test ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

install: fmt vet test build
	@mkdir -p $(PREFIX)/bin
	install -m755 $(BIN) $(PREFIX)/bin/$(BIN)
	@echo "instalado: $(PREFIX)/bin/$(BIN)  ($$($(PREFIX)/bin/$(BIN) version))"

uninstall:
	rm -f $(PREFIX)/bin/$(BIN)

clean:
	rm -f $(BIN)
