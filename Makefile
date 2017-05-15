.DEFAULT_GOAL := all

BUILDPATH := ./build
APPNAME := jsonmon

.PHONY: all install osx linux win clean

define build
	GOOS=$(1) GOARCH=$(2) go build -o $(BUILDPATH)/$(APPNAME)_$(1)_$(2)$(3)
endef

define zip
	cd build && zip $(1)_$(2).zip $(APPNAME)_$(1)_$(2)$(3) && rm $(APPNAME)_$(1)_$(2)$(3)
endef

install:
	go get -u github.com/kardianos/govendor github.com/jteeuwen/go-bindata/...
	go install github.com/kardianos/govendor github.com/jteeuwen/go-bindata/...
	govendor sync -v

osx:
	@echo "Building osx binaries..."
	@$(call build,darwin,amd64,)
	@$(call zip,darwin,amd64,)

linux:
	@echo "Building linux binaries..."
	@$(call build,linux,amd64,)
	@$(call zip,linux,amd64,)

win:
	@echo "Building windows binaries..."
	@$(call build,windows,amd64,.exe)
	@$(call zip,windows,amd64,.exe)

all: install embed-ui osx linux win

embed-ui:
	@echo "Generating bindata..."
	go-bindata -nocompress -nomemcopy -prefix ui/html ui/html

clean:
	@rm -rf build/*
