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

# Goldens: real Planet builds on Livid's Mac, one planet per template,
# fetched into testdata/goldens/<name>/{My,Public} (gitignored). My/
# holds planet.json with API tokens: read here, never committed.
GOLDEN_HOST = livid@192.168.1.99
GOLDEN_ROOT = ~/Library/Containers/xyz.planetable.Planet/Data/Documents/Planet
GOLDENS = sparkle-release:7AB9F0C5-BCAE-4952-A670-18A416D1C968 \
          sparkle-insider:68D9FFF2-0CB2-4790-BFBC-4626B4EEA8ED \
          plain-codelauncher:4107BAAD-A4B5-48A0-8EED-BC57264E513D \
          sepia-drafts:EA6FE2AB-D0BE-47D5-B8C3-E5BF99F082B1 \
          memories-charts:4D65E2EA-076C-434B-AB9D-00582F48CEB4 \
          gamedb-gamedb:A13385B6-D70D-4574-984C-A015C859B33C

goldens:
	@for g in $(GOLDENS); do \
	  name=$${g%%:*}; id=$${g#*:}; \
	  mkdir -p testdata/goldens/$$name; \
	  rsync -a -q -e "ssh -o BatchMode=yes" "$(GOLDEN_HOST):$(GOLDEN_ROOT)/Public/$$id/" testdata/goldens/$$name/Public/ && \
	  rsync -a -q -e "ssh -o BatchMode=yes" "$(GOLDEN_HOST):$(GOLDEN_ROOT)/My/$$id/" testdata/goldens/$$name/My/ && \
	  echo "fetched $$name"; \
	done

.PHONY: goldens
