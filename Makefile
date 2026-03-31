.PHONY: build run clean

# Suppress the duplicate library warning on macOS
export CGO_LDFLAGS=-Wl,-no_warn_duplicate_libraries

build:
	go build -o sonos-status .

# Package for the current platform
package:
	fyne package -icon Icon.png

# Package for specific platforms (requires fyne CLI and cross-compilation setup)
package-macos:
	fyne package -os darwin -icon Icon.png

package-windows:
	fyne package -os windows -icon Icon.png

package-linux:
	fyne package -os linux -icon Icon.png

run:
	go run .

clean:
	rm -f sonos-status
