// Copyright 2025 Daytona Platforms Inc.
// SPDX-License-Identifier: AGPL-3.0

package process

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteTTY_ValidRequest(t *testing.T) {
	// Create a test logger
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer([]byte{}), nil))

	// Create a Gin router with ExecuteTTY handler
	router := gin.New()
	router.POST("/execute-tty", ExecuteTTY(logger))

	// Prepare the request
	request := ExecuteTTYRequest{
		Command: "echo",
		Args:    []string{"hello"},
		Cols:    toUint16Ptr(80),
		Rows:    toUint16Ptr(24),
	}

	reqBody, err := json.Marshal(request)
	require.NoError(t, err)

	// Create HTTP test request
	req, err := http.NewRequest("POST", "/execute-tty", bytes.NewBuffer(reqBody))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	// Record the response
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Verify the response
	assert.Equal(t, http.StatusOK, w.Code)

	var response ExecuteTTYResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.NotEmpty(t, response.SessionID)
	assert.Contains(t, response.SessionID, "ttyexec-")

	// Clean up the TTY session created during the test to avoid resource leaks.
	t.Cleanup(func() {
		if response.SessionID != "" {
			if s, ok := ttyExecSessions.Load(response.SessionID); ok {
				s.(*TTYExecSession).cancel()
			}
			ttyExecSessions.Delete(response.SessionID)
		}
	})
}

func TestExecuteTTY_MissingCommand(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer([]byte{}), nil))

	router := gin.New()
	router.POST("/execute-tty", ExecuteTTY(logger))

	request := ExecuteTTYRequest{
		Command: "", // Empty command
	}

	reqBody, err := json.Marshal(request)
	require.NoError(t, err)

	req, err := http.NewRequest("POST", "/execute-tty", bytes.NewBuffer(reqBody))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should return 400 Bad Request
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "command is required")
}

func TestExecuteTTY_InvalidDimensions(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer([]byte{}), nil))

	router := gin.New()
	router.POST("/execute-tty", ExecuteTTY(logger))

	// Dimensions exceed the maximum
	cols := uint16(2000)
	rows := uint16(1000)

	request := ExecuteTTYRequest{
		Command: "echo hello",
		Cols:    &cols,
		Rows:    &rows,
	}

	reqBody, err := json.Marshal(request)
	require.NoError(t, err)

	req, err := http.NewRequest("POST", "/execute-tty", bytes.NewBuffer(reqBody))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should return 400 Bad Request
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "invalid terminal dimensions")
}

func TestExecuteTTY_DefaultDimensions(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer([]byte{}), nil))

	router := gin.New()
	router.POST("/execute-tty", ExecuteTTY(logger))

	// No dimensions specified - should use defaults
	request := ExecuteTTYRequest{
		Command: "bash",
	}

	reqBody, err := json.Marshal(request)
	require.NoError(t, err)

	req, err := http.NewRequest("POST", "/execute-tty", bytes.NewBuffer(reqBody))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should succeed with default dimensions
	assert.Equal(t, http.StatusOK, w.Code)

	var response ExecuteTTYResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.NotEmpty(t, response.SessionID)

	// Clean up the TTY session created during the test to avoid resource leaks.
	t.Cleanup(func() {
		if response.SessionID != "" {
			if s, ok := ttyExecSessions.Load(response.SessionID); ok {
				s.(*TTYExecSession).cancel()
			}
			ttyExecSessions.Delete(response.SessionID)
		}
	})
}

func TestConnectExecuteTTY_SessionNotFound(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer([]byte{}), nil))

	router := gin.New()
	router.GET("/execute-tty/:sessionId", ConnectExecuteTTY(logger))

	// Try to connect to a non-existent session
	req, err := http.NewRequest("GET", "/execute-tty/nonexistent-session", nil)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should return 404 Not Found
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "TTY execution session not found")
}
