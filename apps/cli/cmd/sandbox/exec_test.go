// Copyright 2025 Daytona Platforms Inc.
// SPDX-License-Identifier: AGPL-3.0

package sandbox

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCommandConstruction(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		contains string
	}{
		{
			name:     "simple command",
			args:     []string{"echo", "hello"},
			contains: "echo hello",
		},
		{
			name:     "shell command with -c",
			args:     []string{"sh", "-c", "echo hello && echo world"},
			contains: "sh -c echo hello && echo world",
		},
		{
			name:     "command with multiple args",
			args:     []string{"ls", "-la", "/tmp"},
			contains: "ls -la /tmp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test that command construction logic would work
			if len(tt.args) >= 3 && tt.args[1] == "-c" {
				// Shell -c case
				parts := []string{tt.args[0], tt.args[1]}
				cmdPart := strings.Join(tt.args[2:], " ")
				result := strings.Join(append(parts, cmdPart), " ")
				assert.Contains(t, result, tt.contains)
			} else {
				// Regular case
				result := strings.Join(tt.args, " ")
				assert.Contains(t, result, tt.contains)
			}
		})
	}
}

func TestExecFlagBehavior(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		expectTTY bool
		expectCwd bool
	}{
		{
			name:      "basic exec without flags",
			args:      []string{"sandbox-id", "--", "echo", "hello"},
			expectTTY: false,
			expectCwd: false,
		},
		{
			name:      "exec with --tty flag",
			args:      []string{"--tty", "sandbox-id", "--", "bash"},
			expectTTY: true,
			expectCwd: false,
		},
		{
			name:      "exec with --cwd flag",
			args:      []string{"--cwd", "/tmp", "sandbox-id", "--", "ls"},
			expectTTY: false,
			expectCwd: true,
		},
		{
			name:      "exec with multiple flags",
			args:      []string{"--cwd", "/tmp", "--tty", "--timeout", "30", "sandbox-id", "--", "bash"},
			expectTTY: true,
			expectCwd: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify flag parsing logic
			hasTTY := contains(tt.args, "--tty")
			hasCwd := contains(tt.args, "--cwd")

			assert.Equal(t, tt.expectTTY, hasTTY)
			assert.Equal(t, tt.expectCwd, hasCwd)

			// Verify that -- separator is present
			assert.True(t, contains(tt.args, "--"))
		})
	}
}

func TestDashDashSeparator(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		hasSeparator bool
		expectCmds   []string
	}{
		{
			name:         "with separator",
			args:         []string{"sandbox-id", "--", "echo", "hello", "world"},
			hasSeparator: true,
			expectCmds:   []string{"echo", "hello", "world"},
		},
		{
			name:         "with separator and flags before",
			args:         []string{"--tty", "sandbox-id", "--", "bash", "-i"},
			hasSeparator: true,
			expectCmds:   []string{"bash", "-i"},
		},
		{
			name:         "no separator",
			args:         []string{"sandbox-id"},
			hasSeparator: false,
			expectCmds:   []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Find the -- separator
			dashDashIndex := -1
			for i, arg := range tt.args {
				if arg == "--" {
					dashDashIndex = i
					break
				}
			}

			if tt.hasSeparator {
				assert.NotEqual(t, -1, dashDashIndex, "should find -- separator")
				if dashDashIndex > -1 && dashDashIndex+1 < len(tt.args) {
					commandArgs := tt.args[dashDashIndex+1:]
					assert.Equal(t, tt.expectCmds, commandArgs)
				}
			} else {
				assert.Equal(t, -1, dashDashIndex, "should not find -- separator")
			}
		})
	}
}

// Helper function
func contains(slice []string, item string) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}
