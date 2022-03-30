#!/usr/bin/make -f

.ONESHELL:
.SHELL := /usr/bin/bash

PROJECTNAME := $(shell basename "$$(pwd)")
PROJECTPATH := $(shell pwd)

help:
	@echo "Usage: make [options] [arguments]\n"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' Makefile | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

install: ## Build the binary and place it in $GOPATH/bin
	@GOPATH=$(GOPATH) GOBIN=$(GOBIN) go install $(LDFLAGS)

run: install ## Install and run the binary providing the benchmark subcommand. All flags are required
# E.g: 'make run filepath=promql_queries.csv workers=1 promscale.url=http://localhost:9201'
	pqlbench benchmark --filepath=$(filepath) --workers=$(workers) --promscale.url=$(promscale.url)

setup: ## Run a script that set-up all that is required to run this project
	bash setup.sh
