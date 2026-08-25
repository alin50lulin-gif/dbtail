package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateConfigurationRejectsMarkdownAPIHost(t *testing.T) {
	opts := GlobalOptions{APIHost: "[http://localhost:8123/](http://localhost:8123/)"}
	if err := validateConfiguration(&opts); err == nil {
		t.Fatal("expected Markdown APIHost to be rejected")
	}
}

func TestValidateConfigurationRejectsEscapedGlob(t *testing.T) {
	opts := GlobalOptions{}
	opts.Reqs.LogFiles = []string{`/var/log/mysql/slow\.log\*`}
	if err := validateConfiguration(&opts); err == nil {
		t.Fatal("expected escaped LogFiles wildcard to be rejected")
	}
}

func TestValidateConfigurationRejectsStateWildcard(t *testing.T) {
	opts := GlobalOptions{}
	opts.Tail.StateFile = "/etc/dbtail/states/*"
	if err := validateConfiguration(&opts); err == nil {
		t.Fatal("expected StateFile wildcard to be rejected")
	}
}

func TestValidateConfigurationCreatesStateDirectory(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "states")
	opts := GlobalOptions{}
	opts.Reqs.LogFiles = []string{"/var/log/mysql/slow.log*"}
	opts.Tail.StateFile = stateDir

	if err := validateConfiguration(&opts); err != nil {
		t.Fatalf("validateConfiguration returned error: %v", err)
	}
	info, err := os.Stat(strings.TrimSuffix(stateDir, string(os.PathSeparator)))
	if err != nil {
		t.Fatalf("state directory was not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("state path is not a directory: %s", stateDir)
	}
}
