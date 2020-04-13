
.PHONY: all
all: fmt build

.PHONY: release
release: fmt release_server release_client release_ws_server release_ws_client

.PHONY: build
build: server client ws_server ws_client

.PHONY: fmt
fmt:
	@go fmt ./...

.PHONY: server
server:
	@go build -ldflags "-w -s" -o bin/$@ cmd/server/main.go

.PHONY: client
client:
	@go build -ldflags "-w -s" -o bin/$@ cmd/client/main.go


.PHONY: ws_server
ws_server:
	@go build -ldflags "-w -s" -o bin/$@ cmd/ws_server/main.go

.PHONY: ws_client
ws_client:
	@go build -ldflags "-w -s" -o bin/$@ cmd/ws_client/main.go

.PHONY: release_server
release_server: server
ifneq ($(UPX_PATH),)
	$(UPX_PATH) -9 bin/server
endif

.PHONY: release_client
release_client: client
ifneq ($(UPX_PATH),)
	$(UPX_PATH) -9 bin/client
endif

.PHONY: release_ws_server
release_ws_server: ws_server
ifneq ($(UPX_PATH),)
	$(UPX_PATH) -9 bin/ws_server
endif

.PHONY: release_ws_client
release_ws_client: ws_client
ifneq ($(UPX_PATH),)
	$(UPX_PATH) -9 bin/ws_client
endif

.PHONY: clean
clean:
	@rm -rf bin
	@go clean

.PHONY: distclean
distclean:
	@rm -rf bin
	@go clean --modcache
