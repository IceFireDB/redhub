# AGENTS.md

This file provides guidance for AI coding agents working on the RedHub codebase.

## Project Overview

RedHub is a high-performance RESP-Server multi-threaded framework, based on RawEpoll model written in Go. It provides a framework for creating Redis-compatible servers with ultra-high performance, full multi-threaded support, and low CPU resource consumption.

## Quick Start

- Go version: Requires Go 1.24.0 (see go.mod)
- Build: `go build`
- Run memory KV example: `go run example/memory_kv/server.go`
- Test: `go test ./...`

## Development Environment

### Dependencies
- `github.com/panjf2000/gnet/v2`: High-performance event-loop networking framework
- `github.com/stretchr/testify`: Testing framework with assertions

### Testing
- Run all tests: `go test ./...`
- Run with coverage: `go test -cover ./...`
- Run with verbose output: `go test -v ./...`
- Run specific package: `go test ./pkg/...`
- Run main package: `go test .`
- Run specific test: `go test -run <TestName> ./...`
- Main test file: `redhub_test.go`

### Code Style
- Follow standard Go conventions (gofmt)
- Add tests for all code changes
- Ensure all tests pass before committing

## Project Structure

```
redhub/
├── redhub.go              # Main package containing the RedHub framework core
├── redhub_test.go         # Tests for the main package
├── pkg/                   # Package directory
│   └── (sub-packages)     # Check contents for specific components
├── example/               # Example implementations
│   └── memory_kv/         # Memory-based key-value store example
├── go.mod                 # Go module definition and dependencies
└── go.sum                 # Go module checksums
```

## Contribution Workflow

1. Create an issue to discuss your change
2. Fork the repository
3. Create a new branch from main/master
4. Make your changes with tests
5. Ensure all tests pass: `go test ./...`
6. Commit with DCO sign-off: `git commit -s -m "message"`
7. Push to your fork
8. Create a pull request

## Commit Guidelines

- Every commit must be signed with DCO (Developer Certificate of Origin)
- Sign automatically: `git commit -s -m "message"`
- Or add manually: `Signed-off-by: Your Name <your.email@example.com>`
- If you forgot to sign: `git commit --amend --no-edit --signoff` then `git push --force-with-lease`
- Write clear, descriptive commit messages

## Pull Request Guidelines

- Reference the related issue in your PR description
- All code changes must include tests
- Wait for CI checks to complete and pass
- Maintainers review and merge within a few days
- Be responsive to review comments

## Performance Considerations

- RedHub uses RawEpoll model for high throughput
- Performance exceeds Redis single-threaded and multi-threaded implementations
- Consider performance impact of any changes
- Use `redis-benchmark` for networking layer performance testing
- Avoid blocking operations in event loop

## RESP Protocol

- Full Redis protocol (RESP) compatibility
- Example commands: SET, GET, DEL, PING, QUIT
- Protocol compliance is critical for client compatibility
- Use official Redis clients for testing

## Common Issues

- Import errors: Ensure Go 1.24.0 is installed
- Test failures: Run `go mod tidy` and check dependencies
- Performance issues: Profile with `pprof` before and after changes
