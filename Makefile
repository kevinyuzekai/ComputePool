.PHONY: build test run tidy clean build-darwin-arm64 build-darwin-amd64 package-dmg package-dmg-amd64 app

VERSION ?= 0.1.1
LDFLAGS := -s -w -X main.version=$(VERSION)

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/computepool ./cmd/computepool

test:
	go test ./...

run: build
	./bin/computepool -open

tidy:
	go mod tidy

clean:
	rm -rf bin build

build-darwin-arm64:
	ARCH=arm64 ./scripts/build-macos.sh

build-darwin-amd64:
	ARCH=amd64 ./scripts/build-macos.sh

package-dmg:
	ARCH=arm64 ./scripts/package-dmg.sh

package-dmg-amd64:
	ARCH=amd64 ./scripts/package-dmg.sh

app: build-darwin-arm64
