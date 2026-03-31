.PHONY: build run clean

# Suppress the duplicate library warning on macOS
export CGO_LDFLAGS=-Wl,-no_warn_duplicate_libraries

build:
	go build -o sonos-status .

run:
	go run .

clean:
	rm -f sonos-status
