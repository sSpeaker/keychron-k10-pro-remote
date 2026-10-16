BINARY = kc
PREFIX = /usr/local/bin

.PHONY: build install clean

build:
	go mod tidy
	go build -o $(BINARY) .

install: build
	install -m 755 $(BINARY) $(PREFIX)/$(BINARY)

clean:
	rm -f $(BINARY) $(BINARY).exe
