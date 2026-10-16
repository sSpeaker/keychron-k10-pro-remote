BINARY = kc
PREFIX = /usr/local/bin
WIN_CC = x86_64-w64-mingw32-gcc

.PHONY: build windows release install clean

build:
	go mod tidy
	go build -o $(BINARY) .
	# Stable identity for Input Monitoring (the Go linker signs it as "a.out")
	codesign -f -s - -i com.sspeaker.kc $(BINARY)

# Cross-compile from macOS: brew install mingw-w64
windows:
	CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=$(WIN_CC) go build -ldflags="-s -w" -o $(BINARY).exe .

# Refresh the prebuilt binaries in ../bin
release: build windows
	cp $(BINARY) ../bin/cli_mac/$(BINARY)
	cp $(BINARY).exe ../bin/cli_win/$(BINARY).exe

install: build
	install -m 755 $(BINARY) $(PREFIX)/$(BINARY)

clean:
	rm -f $(BINARY) $(BINARY).exe
