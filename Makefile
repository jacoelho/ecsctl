.PHONY: lint

objects: $(wildcard *.go)

ecsctl: $(objects)
	go build

lint:
	gofmt -w $$(pwd)
