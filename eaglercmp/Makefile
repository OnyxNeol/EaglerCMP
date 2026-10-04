.PHONY: build test vet dist init launch clean

build:
	scripts/build.sh

test:
	go test ./...

vet:
	go vet ./...

dist:
	scripts/build.sh --all

init: build
	./bin/eaglercmp init

launch: build
	./bin/eaglercmp launch

clean:
	rm -rf bin dist
