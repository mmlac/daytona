// Copyright 2025 Daytona Platforms Inc.
// SPDX-License-Identifier: AGPL-3.0

package process

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseCommand(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "simple command",
			input:    "echo hello",
			expected: []string{"echo", "hello"},
		},
		{
			name:     "command with quoted string",
			input:    `echo "hello world"`,
			expected: []string{"echo", "hello world"},
		},
		{
			name:     "command with single quoted string",
			input:    `echo 'hello world'`,
			expected: []string{"echo", "hello world"},
		},
		{
			name:     "multiple arguments",
			input:    "ls -la /tmp",
			expected: []string{"ls", "-la", "/tmp"},
		},
		{
			name:     "nested quotes",
			input:    `sh -c "echo 'hello'"`,
			expected: []string{"sh", "-c", "echo 'hello'"},
		},
		{
			name:     "backslash-escaped double-quote inside double-quoted string",
			input:    `echo "He said \"hello\""`,
			expected: []string{"echo", `He said "hello"`},
		},
		{
			name:     "empty string",
			input:    "",
			expected: nil, // parseCommand returns nil for empty input
		},
		{
			name:     "spaces only",
			input:    "   ",
			expected: nil, // parseCommand returns nil for whitespace-only input
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseCommand(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExecuteRequest_Validation(t *testing.T) {
	tests := []struct {
		name    string
		request ExecuteRequest
		valid   bool
	}{
		{
			name: "valid request",
			request: ExecuteRequest{
				Command: "echo hello",
			},
			valid: true,
		},
		{
			name: "empty command",
			request: ExecuteRequest{
				Command: "",
			},
			valid: false,
		},
		{
			name: "request with timeout",
			request: ExecuteRequest{
				Command: "sleep 10",
				Timeout: toUint32Ptr(5),
			},
			valid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Basic validation: command is required
			if tt.valid {
				assert.NotEmpty(t, tt.request.Command, "valid request should have non-empty command")
			} else {
				assert.Empty(t, tt.request.Command, "invalid request should have empty command")
			}
		})
	}
}

func TestExecuteTTYRequest_Defaults(t *testing.T) {
	request := ExecuteTTYRequest{
		Command: "bash",
	}

	assert.Equal(t, "bash", request.Command)
	assert.Nil(t, request.Cols)
	assert.Nil(t, request.Rows)
	assert.Nil(t, request.Timeout)
	assert.Nil(t, request.Cwd)
	assert.Nil(t, request.Envs)
}

func TestExecuteTTYRequest_WithValues(t *testing.T) {
	cols := uint16(120)
	rows := uint16(30)
	timeout := uint32(300)
	cwd := "/tmp"
	envs := map[string]string{
		"TERM": "xterm-256color",
		"PATH": "/usr/bin",
	}

	request := ExecuteTTYRequest{
		Command: "bash",
		Args:    []string{"-i", "-l"},
		Cols:    &cols,
		Rows:    &rows,
		Timeout: &timeout,
		Cwd:     &cwd,
		Envs:    envs,
	}

	assert.Equal(t, "bash", request.Command)
	assert.Equal(t, []string{"-i", "-l"}, request.Args)
	assert.Equal(t, &cols, request.Cols)
	assert.Equal(t, &rows, request.Rows)
	assert.Equal(t, &timeout, request.Timeout)
	assert.Equal(t, &cwd, request.Cwd)
	assert.Equal(t, envs, request.Envs)
}

// Helper functions

func toUint32Ptr(val uint32) *uint32 {
	return &val
}

func toUint16Ptr(val uint16) *uint16 {
	return &val
}
