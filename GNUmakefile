.DEFAULT_GOAL := all

BUILDPATH := ./build
APPNAME := jsonmon

.PHONY: all test install osx linux win clean

define build
	GOOS=$(1) GOARCH=$(2) go build -o $(BUILDPATH)/$(APPNAME)_$(1)_$(2)$(3)
endef

define zip
	cd build && zip $(1)_$(2).zip $(APPNAME)_$(1)_$(2)$(3) && rm $(APPNAME)_$(1)_$(2)$(3)
endef

test:
	go test -v -race

install:
	go get -u github.com/jteeuwen/go-bindata/...
	go install github.com/jteeuwen/go-bindata/...

osx:
	@echo "Building osx binaries..."
	@$(call build,darwin,amd64,)
	@$(call zip,darwin,amd64,)

linux:
	@echo "Building linux binaries..."
	@$(call build,linux,amd64,)
	@$(call zip,linux,amd64,)
	@$(call build,linux,386,)
	@$(call zip,linux,386,)

win:
	@echo "Building windows binaries..."
	@$(call build,windows,amd64,.exe)
	@$(call zip,windows,amd64,.exe)

all: install test embed-ui osx linux win

embed-ui:
	@echo "Generating bindata..."
	go-bindata -nocompress -nomemcopy -prefix ui/html ui/html
	go fmt bindata.go

clean:
	@rm -rf build/*
