#!/bin/bash
#
# Conventional Commit Validator
# Validates commit messages against the Conventional Commits specification
# https://www.conventionalcommits.org/
#
# Usage: validate-commit.sh <commit-message-file>
#

set -e

# Conventional commit types
TYPES="feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert"
# Optional scopes (can be customized per project)
SCOPES="[a-zA-Z0-9_-]+"
# Breaking change indicator
BREAKING="!"

# Regex pattern for conventional commits
# Format: <type>[optional scope][!]: <description>
#         [optional body]
#         [optional footer(s)]
PATTERN="^(${TYPES})(\(${SCOPES}\))?(${BREAKING})?: .+"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Get commit message file
COMMIT_MSG_FILE="${1:-.git/COMMIT_EDITMSG}"

if [ ! -f "$COMMIT_MSG_FILE" ]; then
    echo -e "${RED}Error: Commit message file not found: $COMMIT_MSG_FILE${NC}"
    exit 1
fi

# Read commit message (first line only for validation)
COMMIT_MSG=$(head -n 1 "$COMMIT_MSG_FILE")

# Skip validation for merge commits and revert commits
if [[ "$COMMIT_MSG" =~ ^Merge ]] || [[ "$COMMIT_MSG" =~ ^Revert ]]; then
    exit 0
fi

# Validate against pattern
if [[ ! "$COMMIT_MSG" =~ $PATTERN ]]; then
    echo -e "${RED}✗ Invalid commit message format${NC}"
    echo ""
    echo -e "${YELLOW}Your commit message:${NC}"
    echo "  $COMMIT_MSG"
    echo ""
    echo -e "${YELLOW}Expected format:${NC}"
    echo "  <type>[optional scope][!]: <description>"
    echo ""
    echo -e "${YELLOW}Valid types:${NC}"
    echo "  feat     - A new feature"
    echo "  fix      - A bug fix"
    echo "  docs     - Documentation changes"
    echo "  style    - Code style changes (formatting, etc.)"
    echo "  refactor - Code refactoring without feature change"
    echo "  perf     - Performance improvements"
    echo "  test     - Adding or updating tests"
    echo "  build    - Build system or dependency changes"
    echo "  ci       - CI/CD configuration changes"
    echo "  chore    - Maintenance tasks"
    echo "  revert   - Reverting a previous commit"
    echo ""
    echo -e "${YELLOW}Examples:${NC}"
    echo "  feat: add user authentication"
    echo "  fix(storage): resolve database connection issue"
    echo "  docs!: update API documentation (breaking change)"
    echo "  refactor(cli): simplify command parsing"
    echo ""
    exit 1
fi

echo -e "${GREEN}✓ Valid commit message${NC}"
exit 0
