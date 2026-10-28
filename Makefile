# The checks every change passes, the same ones CI runs (.github/workflows/ci.yml).
# The tools, and the versions they are pinned to, are in CONTRIBUTING.md.

GOVULNCHECK := golang.org/x/vuln/cmd/govulncheck@v1.9.0

.PHONY: check test lint fmt vuln

## check: everything below, in the order CI runs it
check: lint test vuln

## test: the server's tests with the race detector, then the page's
test:
	go test -race ./...
	node --test 'web/test/*.test.js'

## lint: vet, golangci-lint for Go, Biome for the page
lint:
	go vet ./...
	golangci-lint run
	biome check web demo biome.jsonc

## fmt: format everything as lint expects
fmt:
	golangci-lint fmt
	biome check --write web demo biome.jsonc

## vuln: known vulnerabilities in the code Overlook calls
vuln:
	go run $(GOVULNCHECK) ./...
