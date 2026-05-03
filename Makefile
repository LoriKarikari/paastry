.PHONY: fmt check test

GO_FILES := $(shell find . -name '*.go' -not -path './gen/*')

fmt:
	@if [ -n "$(GO_FILES)" ]; then gofmt -w $(GO_FILES); fi

check:
	@if [ -n "$(GO_FILES)" ]; then \
		files="$$(gofmt -l $(GO_FILES))"; \
		if [ -n "$$files" ]; then \
			echo "gofmt needed:"; \
			echo "$$files"; \
			exit 1; \
		fi; \
	fi
	@if [ -f go.mod ]; then go test ./...; else echo "go.mod not found; skipping Go tests"; fi

test:
	@if [ -f go.mod ]; then go test ./...; else echo "go.mod not found; skipping Go tests"; fi
