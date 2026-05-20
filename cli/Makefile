BINARY = kc
PREFIX = /usr/local/bin

.PHONY: build install clean

build:
	go get github.com/sstallion/go-hid@latest
	go build -o $(BINARY) .

install: build
	install -m 755 $(BINARY) $(PREFIX)/$(BINARY)

clean:
	rm -f $(BINARY)
