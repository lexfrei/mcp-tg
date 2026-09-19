package main

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"gopkg.in/yaml.v3"
)

const (
	packagingUnit       = "../../packaging/mcp-tg.service"
	packagingEnvExample = "../../packaging/mcp-tg.env.example"
	goreleaserConfig    = "../../.goreleaser.yaml"
)

// envExampleAssignment matches a variable the example file sets, commented
// out or not: a commented line is still an instruction to the reader.
var envExampleAssignment = regexp.MustCompile(`(?m)^#?([A-Z][A-Z0-9_]*)=`)

// TestPackagingEnvExample_NamesRealVariables pins the shipped example
// against the documented environment table, which TestDocsEnvVars_MatchTheCode
// pins against the code in turn. A variable nothing reads sends whoever
// copies this file to configure a no-op, and the file is copied verbatim
// into ~/.mcp-tg/env by the installation docs.
//
// Only this direction is checked: the example configures one deployment (the
// shared daemon) and is not meant to list every variable the server accepts.
func TestPackagingEnvExample_NamesRealVariables(t *testing.T) {
	documented := make(map[string]bool)
	for _, row := range docsEnvRow.FindAllStringSubmatch(readDocsPage(t, docsConfigPage), -1) {
		documented[row[1]] = true
	}

	example, err := os.ReadFile(packagingEnvExample)
	if err != nil {
		t.Fatalf("read %s: %v", packagingEnvExample, err)
	}

	found := envExampleAssignment.FindAllStringSubmatch(string(example), -1)
	if len(found) == 0 || len(documented) == 0 {
		t.Fatalf("found %d assignments and %d documented rows — this test has stopped checking anything",
			len(found), len(documented))
	}

	for _, match := range found {
		if !documented[match[1]] {
			t.Errorf("%s sets %s, which %s does not document — does anything read it?",
				packagingEnvExample, match[1], docsConfigPage)
		}
	}
}

// TestPackagingUnit_StartsTheInstalledBinary pins the unit's ExecStart against
// the path the package actually installs the binary at. The two are set in
// different files and nothing else compares them: a bindir change would leave
// a unit pointing at a path the package no longer writes, and the failure
// surfaces only on a user's machine, as a service that never starts.
func TestPackagingUnit_StartsTheInstalledBinary(t *testing.T) {
	unit, err := os.ReadFile(packagingUnit)
	if err != nil {
		t.Fatalf("read %s: %v", packagingUnit, err)
	}

	execStart := regexp.MustCompile(`(?m)^ExecStart=(\S+)`).FindStringSubmatch(string(unit))
	if execStart == nil {
		t.Fatalf("%s has no ExecStart", packagingUnit)
	}

	bindir, err := nfpmBindir()
	if err != nil {
		t.Fatalf("read %s: %v", goreleaserConfig, err)
	}

	want := strings.TrimSuffix(bindir, "/") + "/mcp-tg"
	if execStart[1] != want {
		t.Errorf("%s starts %s, but the package installs the binary at %s",
			packagingUnit, execStart[1], want)
	}
}

// TestPackagingUnit_IsShippedAsAUserUnit pins that the package still carries
// the unit, and carries it where systemd looks for a packaged user unit.
// /etc/systemd/user belongs to the administrator and ~/.config/systemd/user to
// the user, so shipping into either would overwrite something that is not
// ours; and a unit dropped from contents entirely would leave the docs telling
// people to enable a service that was never installed.
func TestPackagingUnit_IsShippedAsAUserUnit(t *testing.T) {
	contents, err := nfpmContents()
	if err != nil {
		t.Fatalf("read %s: %v", goreleaserConfig, err)
	}

	const want = "/usr/lib/systemd/user/mcp-tg.service"

	for _, entry := range contents {
		if entry.Dst == want {
			if entry.Src != strings.TrimPrefix(packagingUnit, "../../") {
				t.Errorf("%s is installed from %s, expected the tracked unit", want, entry.Src)
			}

			return
		}
	}

	t.Errorf("the deb no longer ships %s", want)
}

type nfpmContent struct {
	Src string `yaml:"src"`
	Dst string `yaml:"dst"`
}

func nfpmConfig() (struct {
	Nfpms []struct {
		Bindir   string        `yaml:"bindir"`
		Contents []nfpmContent `yaml:"contents"`
	} `yaml:"nfpms"`
}, error,
) {
	var config struct {
		Nfpms []struct {
			Bindir   string        `yaml:"bindir"`
			Contents []nfpmContent `yaml:"contents"`
		} `yaml:"nfpms"`
	}

	source, err := os.ReadFile(goreleaserConfig)
	if err != nil {
		return config, errors.Wrap(err, "read config")
	}

	if err := yaml.Unmarshal(source, &config); err != nil {
		return config, errors.Wrap(err, "parse config")
	}

	if len(config.Nfpms) == 0 {
		return config, errors.New("no nfpms block")
	}

	return config, nil
}

// nfpmBindir reports where the package puts the binary, falling back to
// nfpm's own default when the config leaves it unset.
func nfpmBindir() (string, error) {
	config, err := nfpmConfig()
	if err != nil {
		return "", err
	}

	if config.Nfpms[0].Bindir == "" {
		return "/usr/bin", nil
	}

	return config.Nfpms[0].Bindir, nil
}

func nfpmContents() ([]nfpmContent, error) {
	config, err := nfpmConfig()
	if err != nil {
		return nil, err
	}

	return config.Nfpms[0].Contents, nil
}
