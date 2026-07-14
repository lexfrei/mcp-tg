package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strings"
	"testing"
)

// Both build channels inject the version through -X main.<symbol>, and the Go
// linker IGNORES -X against a symbol that does not exist — without a word. So
// renaming these vars would leave every release reporting "dev"/"unknown" over
// the MCP handshake and in Telegram's device list, with nothing failing.
//
// This test reads the symbols the build files actually name and asserts each
// one is still a package-level var here.
func TestLdflagsSymbols_ExistInMain(t *testing.T) {
	declared := packageLevelVars(t, "main.go")

	for _, file := range []string{"../../Containerfile", "../../.goreleaser.yaml"} {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}

		symbols := regexp.MustCompile(`-X main\.(\w+)=`).FindAllStringSubmatch(string(raw), -1)
		if len(symbols) == 0 {
			t.Errorf("%s no longer injects any -X main.<symbol> — the version would silently stay 'dev'", file)

			continue
		}

		for _, match := range symbols {
			if _, ok := declared[match[1]]; !ok {
				t.Errorf("%s injects -X main.%s, but no such package-level var exists — the linker would ignore it silently",
					file, match[1])
			}
		}
	}
}

func packageLevelVars(t *testing.T, filename string) map[string]struct{} {
	t.Helper()

	parsed, err := parser.ParseFile(token.NewFileSet(), filename, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", filename, err)
	}

	vars := make(map[string]struct{})

	for _, decl := range parsed.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}

		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}

			for _, name := range value.Names {
				vars[name.Name] = struct{}{}
			}
		}
	}

	if len(vars) == 0 {
		t.Fatalf("%s declares no package-level vars — did the file move?", strings.TrimSpace(filename))
	}

	return vars
}
