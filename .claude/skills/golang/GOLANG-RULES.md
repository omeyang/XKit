# Go (Golang) Coding Rules

## Usage Guide

- Rules have severity: [C]ritical, [H]igh, [M]edium, [L]ow
- Rules have ID in following format: [Go-{category}{position}-{severity}]
- When rules conflict: Higher severity wins → Existing code patterns take precedence
- Process rules by severity (Critical first)

## Architecture & Structure [A]

- **[Go-A1-C]** Ignore `vendor` directory from editing

## Code Style & Patterns [S]

- **[Go-S1-H]** `else` keyword usage must be avoided. Use early `return/continue/break` instead
- **[Go-S2-C]** Imports must be grouped and ordered:
  1. Standard library
  1. Third-party
  1. First-party (organization packages)
  1. Local (module imports)
- **[Go-S3-H]** Use `golang.org/x/sync/errgroup` for managing concurrent goroutines with error handling
- **[Go-S4-C]** Use `any` instead of `interface{}` for generic types

## Documentation [D]

- **[Go-D1-H]** Document only public interfaces, types, fields, functions, and methods

## Testing Standards [T]

- **[Go-T1-C]** Split success path tests and error path tests. Keep them separate
- **[Go-T2-H]** Use table-driven style when applicable for both success path and error path tests
- **[Go-T3-M]** Use `testify` framework and its `testify/mock`, `testify/require` and `testify/assert` sub-libraries
- **[Go-T4-H]** When test tags are used, make sure they have following format, for example (for unit):
```
//go:build unit
// +build unit
```

## Tools [TT]

- **[Go-TT1-C]** When `golangci-lint` is available, DO NOT change the configuration by any circumstances
