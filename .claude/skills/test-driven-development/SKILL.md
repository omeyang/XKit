---
name: test-driven-development
description: Implement features using Test-Driven Development (TDD) methodology. Use when implementing any new feature, fixing bugs, or refactoring code. Activate when user mentions TDD, "write tests first", "red-green-refactor", or wants a test-first approach.
---

# Test-Driven Development (TDD)

Guide the implementation of features using the Red-Green-Refactor cycle, ensuring high code quality and test coverage.

## When to Use This Skill

Automatically activate when:
- User asks to implement a new feature with tests
- User mentions "TDD", "test first", or "test-driven"
- User wants to fix a bug (regression test first)
- User asks for high test coverage
- Implementing library/package code (like xkit)

## TDD Workflow (Red-Green-Refactor)

### Phase 1: Red (Write Failing Test)

1. **Understand the requirement** - Clarify what behavior needs to be implemented
2. **Write a minimal failing test** that describes the expected behavior
3. **Run the test** - Confirm it fails for the right reason
4. **Commit** (optional): `test: add failing test for <feature>`

```go
// Example: Testing a new function
func TestCalculateTotal_WithValidItems(t *testing.T) {
    items := []Item{{Price: 10}, {Price: 20}}

    total := CalculateTotal(items)

    assert.Equal(t, 30, total)
}
```

### Phase 2: Green (Make Test Pass)

1. **Write minimal code** to make the test pass
2. **Don't over-engineer** - Only implement what's needed
3. **Run tests** - Confirm the test passes
4. **Commit**: `feat: implement <feature>`

```go
// Minimal implementation
func CalculateTotal(items []Item) int {
    total := 0
    for _, item := range items {
        total += item.Price
    }
    return total
}
```

### Phase 3: Refactor (Improve Code)

1. **Clean up** - Remove duplication, improve naming
2. **Keep tests green** - Run tests after each change
3. **Commit**: `refactor: improve <component>`

## Go-Specific TDD Guidelines

### Table-Driven Tests

Always prefer table-driven tests for Go:

```go
func TestCalculateTotal(t *testing.T) {
    tests := []struct {
        name     string
        items    []Item
        expected int
    }{
        {"empty list", nil, 0},
        {"single item", []Item{{Price: 10}}, 10},
        {"multiple items", []Item{{Price: 10}, {Price: 20}}, 30},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := CalculateTotal(tt.items)
            assert.Equal(t, tt.expected, got)
        })
    }
}
```

### Test Coverage Guidelines

| Package Type | Minimum Coverage |
|-------------|------------------|
| `pkg/` (public API) | 80%+ |
| `internal/` | 70%+ |
| Critical paths | 90%+ |

### Testing Patterns for xkit

1. **Unit tests**: Test individual functions in isolation
2. **Integration tests**: Test component interactions
3. **Example tests**: Provide documentation via `ExampleXxx` functions
4. **Benchmark tests**: For performance-critical code

```go
// Example test for documentation
func ExampleCalculateTotal() {
    items := []Item{{Price: 10}, {Price: 20}}
    total := CalculateTotal(items)
    fmt.Println(total)
    // Output: 30
}
```

## Bug Fix Workflow

When fixing bugs, always:

1. **Write a failing test** that reproduces the bug
2. **Fix the bug** with minimal changes
3. **Verify test passes**
4. **Commit**: `fix: <description of bug fix>`

## Commands

Run tests:
```bash
go test ./... -v
```

Run with coverage:
```bash
go test ./... -cover -coverprofile=coverage.out
go tool cover -html=coverage.out
```

Run specific test:
```bash
go test -run TestCalculateTotal ./pkg/...
```

## Integration with Other Skills

- Use with **golang** skill for idiomatic Go code
- Use with **test-fixing** skill when tests fail
- Use with **code-reviewer** skill for quality checks
