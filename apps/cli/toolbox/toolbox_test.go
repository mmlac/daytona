// Copyright Daytona Platforms Inc.
// SPDX-License-Identifier: AGPL-3.0

package toolbox

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExecuteTTYRequestMarshaling(t *testing.T) {
	cols := uint16(100)
	rows := uint16(40)
	timeout := uint32(300)
	cwd := "/tmp"
	envs := map[string]string{
		"TERM": "xterm-256color",
		"USER": "testuser",
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

	// Marshal to JSON
	data, err := json.Marshal(request)
	assert.NoError(t, err)

	// Unmarshal back
	var unmarshaled ExecuteTTYRequest
	err = json.Unmarshal(data, &unmarshaled)
	assert.NoError(t, err)

	// Verify all fields are preserved
	assert.Equal(t, request.Command, unmarshaled.Command)
	assert.Equal(t, request.Args, unmarshaled.Args)
	assert.Equal(t, request.Cols, unmarshaled.Cols)
	assert.Equal(t, request.Rows, unmarshaled.Rows)
	assert.Equal(t, request.Timeout, unmarshaled.Timeout)
	assert.Equal(t, request.Cwd, unmarshaled.Cwd)
	assert.Equal(t, request.Envs, unmarshaled.Envs)
}

func TestExecuteTTYResponseMarshaling(t *testing.T) {
	response := ExecuteTTYResponse{
		SessionID: "ttyhexec-1234567890",
	}

	// Marshal to JSON
	data, err := json.Marshal(response)
	assert.NoError(t, err)

	// Unmarshal back
	var unmarshaled ExecuteTTYResponse
	err = json.Unmarshal(data, &unmarshaled)
	assert.NoError(t, err)

	assert.Equal(t, response.SessionID, unmarshaled.SessionID)
}

func TestExecuteRequestWithTTYFlag(t *testing.T) {
	ttyFlag := true

	request := ExecuteRequest{
		Command: "echo hello",
		TTY:     &ttyFlag,
	}

	// Marshal to JSON
	data, err := json.Marshal(request)
	assert.NoError(t, err)

	// Unmarshal back
	var unmarshaled ExecuteRequest
	err = json.Unmarshal(data, &unmarshaled)
	assert.NoError(t, err)

	assert.Equal(t, request.Command, unmarshaled.Command)
	assert.NotNil(t, unmarshaled.TTY)
	assert.True(t, *unmarshaled.TTY)
}

func TestExecuteRequestWithoutTTYFlag(t *testing.T) {
	request := ExecuteRequest{
		Command: "echo hello",
	}

	// Marshal to JSON
	data, err := json.Marshal(request)
	assert.NoError(t, err)

	// Unmarshal back
	var unmarshaled ExecuteRequest
	err = json.Unmarshal(data, &unmarshaled)
	assert.NoError(t, err)

	assert.Equal(t, request.Command, unmarshaled.Command)
	assert.Nil(t, unmarshaled.TTY)
}

func TestExecuteTTYRequestDefaults(t *testing.T) {
	request := ExecuteTTYRequest{
		Command: "bash",
	}

	assert.Equal(t, "bash", request.Command)
	assert.Nil(t, request.Args)
	assert.Nil(t, request.Cols)
	assert.Nil(t, request.Rows)
	assert.Nil(t, request.Timeout)
	assert.Nil(t, request.Cwd)
	assert.Nil(t, request.Envs)
}
func TestIsControlMessage(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected bool
	}{
		{
			name:     "valid control message",
			input:    []byte(`{"type":"control","status":"connected"}`),
			expected: true,
		},
		{
			name:     "exit control message",
			input:    []byte(`{"type":"control","status":"exited","exitCode":0}`),
			expected: true,
		},
		{
			name:     "non-control type",
			input:    []byte(`{"type":"data","payload":"hello"}`),
			expected: false,
		},
		{
			name:     "invalid JSON",
			input:    []byte(`not json`),
			expected: false,
		},
		{
			name:     "JSON object without type=control",
			input:    []byte(`{"looks":"like json but isn't control"}`),
			expected: false,
		},
		{
			name:     "empty input",
			input:    []byte{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isControlMessage(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseControlMessage(t *testing.T) {
	tests := []struct {
		name           string
		input          []byte
		expectedCode   int
		expectedIsExit bool
	}{
		{
			name:           "clean exit code 0",
			input:          []byte(`{"type":"control","status":"exited","exitCode":0}`),
			expectedCode:   0,
			expectedIsExit: true,
		},
		{
			name:           "non-zero exit code",
			input:          []byte(`{"type":"control","status":"exited","exitCode":1}`),
			expectedCode:   1,
			expectedIsExit: true,
		},
		{
			name:           "large exit code SIGKILL",
			input:          []byte(`{"type":"control","status":"exited","exitCode":137}`),
			expectedCode:   137,
			expectedIsExit: true,
		},
		{
			name:           "connected status is not exit",
			input:          []byte(`{"type":"control","status":"connected"}`),
			expectedCode:   0,
			expectedIsExit: false,
		},
		{
			name:           "missing exitCode defaults to 0",
			input:          []byte(`{"type":"control","status":"exited"}`),
			expectedCode:   0,
			expectedIsExit: true,
		},
		{
			name:           "invalid JSON returns false",
			input:          []byte(`not json`),
			expectedCode:   0,
			expectedIsExit: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, isExit := parseControlMessage(tt.input)
			assert.Equal(t, tt.expectedIsExit, isExit)
			if tt.expectedIsExit {
				assert.Equal(t, tt.expectedCode, code)
			}
		})
	}
}
