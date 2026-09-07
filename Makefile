.PHONY: build test vet fmt lint clean render

build:
	go build ./...

test:
	go test ./... -v

vet:
	go vet ./...

fmt:
	gofmt -l -w .
	go run github.com/codetesla51/logos@v0.4.10 fmt examples/*.lgv examples/helpers/*.lgv

lint:
	golangci-lint run

render:
	go run ./cmd/kyfram render examples/intro_voice.lgv --out out.mp4

view:
	go run ./cmd/kyfram view examples/intro_voice.lgv

clean:
	rm -f out.mp4 intro_voice.mp4 /tmp/*.mp4
