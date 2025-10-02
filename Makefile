# Variables
GO          := go
TESTFILE    := _testok

# go tools versions
GOLANGCI=github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0
gofumpt=mvdan.cc/gofumpt@latest
govulncheck=golang.org/x/vuln/cmd/govulncheck@latest
goimports=golang.org/x/tools/cmd/goimports@latest

# Default target
.PHONY: default
default: lint

.PHONY: help
help: ## Show the available commands
	@grep -E '^[0-9a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'


.PHONY: lint
lint: fmt vulncheck ## Run linters on all go files
	$(GO) run $(GOLANGCI) run -v

.PHONY: lint-novuln
lint-novuln: fmt ## Run linters on all go files
	$(GO) run $(GOLANGCI) run -v

.PHONY: vulncheck
vulncheck: ## Check for vulnerabilities in dependencies
	$(GO) run $(govulncheck) ./...

.PHONY: fmt
fmt: ## Formats all go files
	go mod tidy
	$(GO) run $(gofumpt) -l -w -extra  .

