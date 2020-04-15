
_UPX_ENV ?= --no-env
_UPX ?= $(shell which upx)

ifneq ($(UPX),)
_UPX := $(shell [ -f $(UPX) ] && echo $(UPX) || echo $(UPX)/upx)
endif

ifneq ($(_UPX),)
_UPX := $(shell [ -x $(_UPX) ] && echo $(_UPX) || which upx)
endif

ifeq ($(UPX_FAST),)
_UPX_ENV += --ultra-brute -9
else
_UPX_ENV += -1
endif

.PHONY: all
all: build

.PHONY: build
build: server client ws_server ws_client

.PHONY: release
release: fmt release_server release_client release_ws_server release_ws_client

.PHONY: release_build
release_build:
ifneq ($(BINNAME),)
	@rm -rf release/$(BINNAME)
ifneq ($(_UPX),)
	@$(_UPX) $(_UPX_ENV) bin/$(BINNAME) -o release/$(BINNAME)
else
	@echo -e "\033[32;1m### \033[31;1mNo UPX be found, Uncompressed provided!\033[32;1m ###\033[0m"
	@cp -raf bin/$(BINNAME) release/$(BINNAME)
endif
endif

.PHONY: init
init:
	@mkdir -p bin release

.PHONY: fmt
fmt:
	@go fmt ./...

.PHONY: server
server:
	@go build -ldflags "-w -s" -o bin/$@ cmd/$@/main.go

.PHONY: client
client:
	@go build -ldflags "-w -s" -o bin/$@ cmd/$@/main.go

.PHONY: ws_server
ws_server:
	@go build -ldflags "-w -s" -o bin/$@ cmd/$@/main.go

.PHONY: ws_client
ws_client:
	@go build -ldflags "-w -s" -o bin/$@ cmd/$@/main.go

.PHONY: release_server
release_server: server
	@BINNAME=$^ make -C . release_build

.PHONY: release_client
release_client: client
	@BINNAME=$^ make -C . release_build

.PHONY: release_ws_server
release_ws_server: ws_server
	@BINNAME=$^ make -C . release_build

.PHONY: release_ws_client
release_ws_client: ws_client
	@BINNAME=$^ make -C . release_build

.PHONY: clean
clean:
	@go clean -i -n -x -cache
	@rm -rf bin go.sum

.PHONY: distclean
distclean:
	@go clean -i -n -x --modcache
	@rm -rf bin go.sum
