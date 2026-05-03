.PHONY: fmt check test vet vuln lint

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
	@if [ -f go.mod ]; then go vet ./...; else echo "go.mod not found; skipping go vet"; fi

test:
	@if [ -f go.mod ]; then go test ./...; else echo "go.mod not found; skipping Go tests"; fi

vet:
	@if [ -f go.mod ]; then go vet ./...; else echo "go.mod not found; skipping go vet"; fi

vuln:
	@if [ -f go.mod ]; then govulncheck ./...; else echo "go.mod not found; skipping govulncheck"; fi

lint:
	@if [ -f go.mod ]; then golangci-lint run ./...; else echo "go.mod not found; skipping golangci-lint"; fi
