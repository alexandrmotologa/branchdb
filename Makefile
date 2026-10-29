.PHONY: all build test lint clean web-install web-build

BINARY_NAME=branchdb

all: build

web-install:
	cd web && npm install

web-build: web-install
	cd web && npm run build

build: web-build
	go build -o bin/$(BINARY_NAME) ./cmd/branchdb

test:
	go test -v -timeout 60s ./...

bench:
	go test -v -run TestPerformanceBenchmarks ./test/...

clean:
	rm -rf bin/ internal/server/dist/ web/dist/ web/node_modules/
