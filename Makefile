.PHONY: init run lint test build clean

init:
	@cp -n configs/config.example.yaml configs/config.yaml 2>/dev/null || echo "✅ config.yaml already exists"

run: init
	go run ./cmd/aigw

lint:
	golangci-lint run

test:
	go test -race -v ./...

build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/aigw ./cmd/aigw

clean:
	rm -rf bin/