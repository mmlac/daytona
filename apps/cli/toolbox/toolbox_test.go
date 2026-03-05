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
