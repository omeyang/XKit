//go:build ignore

// Run from the repository root with go run tests/check-go-size.go.
package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	functionLimit = 150
	fileTarget    = 800
	fileLimit     = 820
)

func main() {
	failed := false
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != "." && (strings.HasPrefix(entry.Name(), ".") || entry.Name() == "vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		invalid, err := checkFile(path)
		failed = failed || invalid
		return err
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if failed {
		os.Exit(1)
	}
}

func checkFile(path string) (bool, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	lines := bytes.Count(src, []byte{'\n'})
	if len(src) > 0 && src[len(src)-1] != '\n' {
		lines++
	}
	failed := lines > fileLimit
	if lines > fileTarget {
		fmt.Printf("%s: %d 行（目标 %d，上限 %d）\n", path, lines, fileTarget, fileLimit)
	}
	if strings.HasSuffix(path, "_test.go") && !strings.HasPrefix(filepath.ToSlash(path), "tests/") {
		fmt.Printf("%s: 测试源码应放入 tests/\n", path)
		failed = true
	}
	positions := token.NewFileSet()
	file, err := parser.ParseFile(positions, path, src, parser.SkipObjectResolution)
	if err != nil {
		return false, err
	}
	ast.Inspect(file, func(node ast.Node) bool {
		var name string
		switch fn := node.(type) {
		case *ast.FuncDecl:
			name = fn.Name.Name
		case *ast.FuncLit:
			name = "函数字面量"
		default:
			return true
		}
		start := positions.PositionFor(node.Pos(), false).Line
		length := positions.PositionFor(node.End(), false).Line - start + 1
		if length > functionLimit {
			fmt.Printf("%s:%d: %s 共 %d 行，上限 %d\n", path, start, name, length, functionLimit)
			failed = true
		}
		return true
	})
	return failed, nil
}
