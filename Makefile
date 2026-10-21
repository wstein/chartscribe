# SPDX-License-Identifier: EUPL-1.2
.DEFAULT_GOAL := help

.PHONY: help fmt fmt-check vet test check

help:
	@printf '%s\n' \
	  'make check   Formatting check, go vet and go tests (read-only)' \
	  'make fmt     Format Go sources (writes files)' \
	  'make test    go test ./...' \
	  'make vet     go vet ./...' \
	  'No build, release or publication targets.'

fmt:
	gofmt -w cmd internal

fmt-check:
	@files=$$(gofmt -l cmd internal) || exit $$?; \
	  if test -n "$$files"; then printf 'Run make fmt:\n%s\n' "$$files"; exit 1; fi

vet:
	go vet ./...

test:
	go test ./...

check: fmt-check vet test
