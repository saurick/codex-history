.PHONY: build test index serve

build:
	go build -o ./codex-history ./cmd/codex-history

test:
	go test ./...

index:
	go run ./cmd/codex-history index

serve:
	go run ./cmd/codex-history serve
