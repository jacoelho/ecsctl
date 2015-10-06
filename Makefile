
VERSION ?= 1.0.0
BINARY = ecsctl
LDFLAGS=-ldflags "-X main.Version=${VERSION}"
ARQUITECTURES = darwin linux
TARGETS = $(addprefix $(BINARY)_, $(ARQUITECTURES))

.PHONY: $(TARGETS) lint

all: $(TARGETS)

lint:
	gofmt -w $$(pwd)

$(TARGETS): lint
		$(foreach TARGET, $(ARQUITECTURES), \
			GOOS=$(TARGET) go build $(LDFLAGS) -o ${BINARY}_$(TARGET); \
		)
