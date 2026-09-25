GO ?= /usr/local/go/bin/go
UNIT = $(HOME)/.config/systemd/user/exe-planet.service

build:
	CGO_ENABLED=0 $(GO) build -o exe-planet ./cmd/exe-planet

test:
	$(GO) test ./...

# Install the user unit once; afterwards `make restart` is the loop.
install: build
	mkdir -p $(dir $(UNIT))
	cp exe-planet.service $(UNIT)
	XDG_RUNTIME_DIR=/run/user/$(shell id -u) systemctl --user daemon-reload
	XDG_RUNTIME_DIR=/run/user/$(shell id -u) systemctl --user enable --now exe-planet

restart: build
	XDG_RUNTIME_DIR=/run/user/$(shell id -u) systemctl --user restart exe-planet

.PHONY: build test install restart
