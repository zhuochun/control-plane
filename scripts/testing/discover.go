// Command discover lists top-level Go test families using the Go parser.
// It intentionally does not classify tests: the maintained index owns that decision.
package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type testCase struct {
	File string `json:"file"`
	Name string `json:"name"`
}

func main() {
	var cases []testCase
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "dist", "test-results", "test-output":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if ok && function.Recv == nil && (strings.HasPrefix(function.Name.Name, "Example") || strings.HasPrefix(function.Name.Name, "Fuzz")) {
				return fmt.Errorf("unsupported runnable Go family %s in %s: register an explicit runner before adoption", function.Name.Name, path)
			}
			if ok && function.Recv == nil && strings.HasPrefix(function.Name.Name, "Test") {
				cases = append(cases, testCase{File: filepath.ToSlash(path), Name: function.Name.Name})
			}
		}
		return nil
	})
	if err == nil {
		err = json.NewEncoder(os.Stdout).Encode(cases)
	}
	if err != nil {
		panic(err)
	}
}
