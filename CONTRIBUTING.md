# Contributing to Babylon Tower

This document provides technical specifications and guidelines for building, testing, and contributing to Babylon Tower.

## Requirements

- Go 1.25 or later
- GNU Make
- protoc (optional, for protobuf generation)

## Quick Start

### Build

```bash
# Build the application
make build

# The binary will be created at at ./bin/messenger
```

### Check Version

```bash
# Show version information
make version
```

### Run Tests

```bash
# Run all tests
make test

# Run tests with coverage report
make test-coverage
```

### Run Linter

```bash
# Install linter first (if not already installed)
make install-deps

# Run linter
make lint
```

### Run the Application

```bash
# Build and run
make run
```

## Makefile Commands

| Command | Description |
|---------|-------------|
| `make build` | Build the application |
| `make version` | Show version information |
| `make test` | Run all tests |
| `make test-coverage` | Run tests with coverage report |
| `make lint` | Run golangci-lint |
| `make fmt` | Format code |
| `make vet` | Run go vet |
| `make proto` | Generate protobuf code |
| `make tidy` | Tidy go modules |
| `make clean` | Clean build artifacts |
| `make run` | Build and run the application |
| `make help` | Show all available commands |
| `make install-hooks` | Install git hooks for commit validation |
| `make uninstall-hooks` | Uninstall git hooks |

## Commit Guidelines

This project follows the [Conventional Commits](https://www.conventionalcommits.org/) specification.

### Format

```
<type>[optional scope][!]: <description>
```

### Valid Types

| Type | Description |
|------|-------------|
| `feat` | A new feature |
| `fix` | A bug fix |
| `docs` | Documentation changes |
| `style` | Code style changes (formatting, etc.) |
| `refactor` | Code refactoring without feature change |
| `perf` | Performance improvements |
| `test` | Adding or updating tests |
| `build` | Build system or dependency changes |
| `ci` | CI/CD configuration changes |
| `chore` | Maintenance tasks |
| `revert` | Reverting a previous commit |

### Examples

```
feat: add user authentication
fix(storage): resolve database connection issue
docs!: update API documentation (breaking change)
refactor(cli): simplify command parsing
```

### Installing Git Hooks

To enforce commit message validation locally:

```bash
make install-hooks
```

This will configure git to use the hooks in `.githooks/` which validate commit messages before each commit.
