# ACTX CLI Makefile
# 主要目标: build / run / test / vet / fmt / clean

BINARY   := actx
PKG      := ./...
MAIN     := ./cmd/actx

.PHONY: all build run test vet fmt tidy clean install help

all: fmt vet build

## build: 编译 ACTX 可执行文件 (默认生成 actx.exe)
build:
	go build -o $(BINARY).exe $(MAIN)

## run: 以交互模式运行 ACTX
run:
	go run $(MAIN)

## test: 运行全部单元测试
test:
	go test $(PKG)

## vet: 运行 go vet 静态检查
vet:
	go vet $(PKG)

## fmt: 格式化所有 Go 源码
fmt:
	gofmt -w $(shell find . -name '*.go' -not -path './vendor/*')

## tidy: 整理 go.mod / go.sum
tidy:
	go mod tidy

## clean: 删除编译产物
clean:
	rm -f $(BINARY).exe

## install: 安装到 GOPATH/bin
install:
	go install $(MAIN)

## help: 显示所有目标
help:
	@echo "可用目标:"
	@grep -E '^## ' Makefile | sed 's/## //'
