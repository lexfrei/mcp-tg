package main

import (
	"bufio"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var (
	readmeToolBullet = regexp.MustCompile("^- `(tg_[a-z0-9_]+)`")
	readmeToolsTitle = regexp.MustCompile(`^## Tools \((\d+)\)`)
)

// readmeToolSection parses README's "## Tools" section and returns the
// count claimed in the heading plus every tool name documented as a
// bullet. Bullets outside the section (e.g. the peer-identifier notes)
// must not count — that is exactly how the documented numbers drifted
// by five without anyone noticing.
func readmeToolSection(t *testing.T) (int, []string) {
	t.Helper()

	file, err := os.Open("../../README.md")
	if err != nil {
		t.Fatalf("open README: %v", err)
	}
	defer file.Close()

	var (
		claimed int
		names   []string
		inTools bool
	)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "## ") {
			match := readmeToolsTitle.FindStringSubmatch(line)
			inTools = match != nil

			if match != nil {
				claimed, _ = strconv.Atoi(match[1])
			}

			continue
		}

		if !inTools {
			continue
		}

		if match := readmeToolBullet.FindStringSubmatch(line); match != nil {
			names = append(names, match[1])
		}
	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("scan README: %v", err)
	}

	return claimed, names
}

// TestReadmeToolList_MatchesRegisteredTools pins the README tool list
// against the registered server: every registered tool must be
// documented, no documented tool may be stale, and the heading count
// must match reality.
func TestReadmeToolList_MatchesRegisteredTools(t *testing.T) {
	registered := listRegisteredTools(t)

	registeredNames := make(map[string]bool, len(registered))
	for _, tool := range registered {
		registeredNames[tool.Name] = true
	}

	claimed, documented := readmeToolSection(t)

	if claimed != len(registered) {
		t.Errorf("README heading claims %d tools, server registers %d", claimed, len(registered))
	}

	documentedNames := make(map[string]bool, len(documented))
	for _, name := range documented {
		if documentedNames[name] {
			t.Errorf("README documents %s twice", name)
		}

		documentedNames[name] = true

		if !registeredNames[name] {
			t.Errorf("README documents %s, but no such tool is registered", name)
		}
	}

	for name := range registeredNames {
		if !documentedNames[name] {
			t.Errorf("registered tool %s is missing from the README tool list", name)
		}
	}
}
