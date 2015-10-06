
VERSION ?= 1.0.0
BINARY = ecsctl
SOURCEDIR = .
SOURCES := $(shell find $(SOURCEDIR) -name '*.go')
LDFLAGS=-ldflags "-X main.Version=${VERSION}"

.PHONY: $(BINARY) lint

all: $(BINARY)

lint:
	gofmt -w $$(pwd)

$(BINARY): lint
	go build $(LDFLAGS) -o ${BINARY}
