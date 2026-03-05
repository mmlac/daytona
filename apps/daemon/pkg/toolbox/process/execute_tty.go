// Copyright 2025 Daytona Platforms Inc.
// SPDX-License-Identifier: AGPL-3.0

package process

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/daytonaio/daemon/internal/util"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// TTYExecSession represents a single TTY execution session
type TTYExecSession struct {
	logger *slog.Logger

	sessionID string
	cmd       *exec.Cmd
	ptmx      *os.File
	request   ExecuteTTYRequest

	ctx    context.Context
	cancel context.CancelFunc

	clients   map[string]*ttyWSClient
	clientsMu sync.RWMutex

	// funnel of all client inputs -> single PTY writer (preserves ordering)
	inCh chan []byte

	// guards general session fields (cmd/ptmx)
	mu sync.Mutex

	exitCode   int
	exitReason string
	finished   bool
	finishedMu sync.Mutex
}

// ttyWSClient represents a WebSocket client connection for TTY exec
type ttyWSClient struct {
	id        string
	conn      *websocket.Conn
	send      chan []byte
	closeOnce sync.Once
}

var ttyExecSessions = &sync.Map{} // sessionID -> *TTYExecSession

// ExecuteTTY godoc
//
//	@Summary		Execute a command with TTY support via WebSocket
//	@Description	Execute a command in a pseudo-terminal and return WebSocket connection details
//	@Tags			process
//	@Accept			json
//	@Produce		json
//	@Param			request	body		ExecuteTTYRequest	true	"TTY command execution request"
//	@Success		200		{object}	ExecuteTTYResponse
//	@Router			/process/execute-tty [post]
//
//	@id				ExecuteTTY
func ExecuteTTY(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request ExecuteTTYRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if request.Command == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "command is required"})
			return
		}

		// Set defaults
		if request.Cols == nil {
			request.Cols = util.Pointer(uint16(80))
		}
		if request.Rows == nil {
			request.Rows = util.Pointer(uint16(24))
		}
		if *request.Cols > 1000 || *request.Rows > 1000 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid terminal dimensions"})
			return
		}
		if request.Envs == nil {
			request.Envs = make(map[string]string)
		}
		if _, hasTerm := request.Envs["TERM"]; !hasTerm {
			request.Envs["TERM"] = "xterm-256color"
		}

		// Generate a session ID
		sessionID := fmt.Sprintf("ttyhexec-%d", time.Now().UnixNano())

		// Create and start the TTY session
		session := &TTYExecSession{
			logger:    logger.With(slog.String("sessionId", sessionID)),
			sessionID: sessionID,
			request:   request,
			clients:   make(map[string]*ttyWSClient),
			inCh:      make(chan []byte, 1024),
		}

		// Start the session
		if err := session.start(); err != nil {
			session.logger.Error("failed to start TTY exec session", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start TTY session"})
			return
		}

		// Store the session
		ttyExecSessions.Store(sessionID, session)

		c.JSON(http.StatusOK, ExecuteTTYResponse{SessionID: sessionID})
	}
}

// ConnectExecuteTTY godoc
//
//	@Summary		Connect to TTY execution session via WebSocket
//	@Description	Establish a WebSocket connection to interact with a TTY execution session
//	@Tags			process
//	@Param			sessionId	path	string	true	"TTY execution session ID"
//	@Success		101			"Switching Protocols - WebSocket connection established"
//	@Router			/process/execute-tty/{sessionId} [get]
//
//	@id				ConnectExecuteTTY
func ConnectExecuteTTY(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := c.Param("sessionId")
		if sessionID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "session ID is required"})
			return
		}

		// Get the session
		sessionVal, ok := ttyExecSessions.Load(sessionID)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "TTY execution session not found"})
			return
		}
		session := sessionVal.(*TTYExecSession)

		// Upgrade to WebSocket
		ws, err := util.UpgradeToWebSocket(c.Writer, c.Request)
		if err != nil {
			logger.Error("WebSocket upgrade failed", "error", err, "sessionId", sessionID)
			return
		}

		session.attachWebSocket(ws)
	}
}

// start initializes and starts the TTY exec session
func (s *TTYExecSession) start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	s.ctx = ctx
	s.cancel = cancel

	// Parse the command or use command + args
	var cmd *exec.Cmd
	if len(s.request.Args) > 0 {
		cmd = exec.CommandContext(ctx, s.request.Command, s.request.Args...)
	} else {
		// Parse command string (legacy support)
		cmdParts := parseCommand(s.request.Command)
		if len(cmdParts) == 0 {
			cancel()
			return errors.New("empty command")
		}
		cmd = exec.CommandContext(ctx, cmdParts[0], cmdParts[1:]...)
	}

	// Set working directory
	if s.request.Cwd != nil {
		cmd.Dir = *s.request.Cwd
	}

	// Set environment
	cmd.Env = os.Environ()
	for k, v := range s.request.Envs {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Start PTY
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Rows: *s.request.Rows,
		Cols: *s.request.Cols,
	})
	if err != nil {
		cancel()
		return fmt.Errorf("failed to start PTY: %w", err)
	}

	s.cmd = cmd
	s.ptmx = ptmx

	s.logger.Debug("Started TTY exec session", "command", s.request.Command, "pid", cmd.Process.Pid)

	// Output reader loop
	go s.ptyReadLoop()

	// Input writer loop
	go s.inputWriteLoop()

	// Process reaper
	go func() {
		err := s.cmd.Wait()
		var exitCode int
		var exitReason string

		if err != nil {
			if exitError, ok := err.(*exec.ExitError); ok {
				exitCode = exitError.ExitCode()
				if exitCode == 137 {
					exitReason = " (SIGKILL)"
				} else if exitCode == 130 {
					exitReason = " (SIGINT - Ctrl+C)"
				} else if exitCode == 143 {
					exitReason = " (SIGTERM)"
				} else if exitCode > 128 {
					sigNum := exitCode - 128
					exitReason = fmt.Sprintf(" (signal %d)", sigNum)
				} else {
					exitReason = " (non-zero exit)"
				}
			} else {
				exitCode = 1
				exitReason = " (process error)"
			}
		} else {
			exitCode = 0
			exitReason = " (clean exit)"
		}

		s.finishedMu.Lock()
		s.finished = true
		s.exitCode = exitCode
		s.exitReason = exitReason
		s.finishedMu.Unlock()

		s.closeClientsWithExitCode(exitCode, exitReason)

		// Clean up session
		ttyExecSessions.Delete(s.sessionID)
		s.logger.Debug("TTY exec session exited", "exitCode", exitCode, "exitReason", exitReason)
	}()

	return nil
}

// attachWebSocket adds a WebSocket client and starts handling messages
func (s *TTYExecSession) attachWebSocket(conn *websocket.Conn) {
	clientID := fmt.Sprintf("client-%d", time.Now().UnixNano())
	client := &ttyWSClient{
		id:   clientID,
		conn: conn,
		send: make(chan []byte, 256),
	}

	s.clientsMu.Lock()
	s.clients[clientID] = client
	s.clientsMu.Unlock()

	// Send control message
	s.mu.Lock()
	finished := s.finished
	s.mu.Unlock()

	if finished {
		// Send exit code immediately
		exitMsg := map[string]interface{}{
			"type":     "control",
			"status":   "exited",
			"exitCode": s.exitCode,
			"reason":   s.exitReason,
		}
		if b, err := json.Marshal(exitMsg); err == nil {
			_ = conn.WriteMessage(websocket.TextMessage, b)
		}
		_ = conn.Close()
		s.clientsMu.Lock()
		delete(s.clients, clientID)
		s.clientsMu.Unlock()
		return
	}

	// Send connected control message
	connMsg := map[string]interface{}{
		"type":   "control",
		"status": "connected",
	}
	if b, err := json.Marshal(connMsg); err == nil {
		_ = conn.WriteMessage(websocket.TextMessage, b)
	}

	// Handle reads and writes
	go s.handleWebSocketReads(client)
	go s.handleWebSocketWrites(client)
}

// handleWebSocketReads reads from WebSocket and sends to PTY
func (s *TTYExecSession) handleWebSocketReads(client *ttyWSClient) {
	defer func() {
		s.clientsMu.Lock()
		delete(s.clients, client.id)
		s.clientsMu.Unlock()
		_ = client.conn.Close()
	}()

	client.conn.SetReadDeadline(time.Now().Add(time.Hour * 24))
	client.conn.SetReadLimit(512 * 1024)

	for {
		_, data, err := client.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				s.logger.Debug("WebSocket read error", "clientId", client.id, "error", err)
			}
			return
		}

		// Try to send to input channel (non-blocking)
		select {
		case s.inCh <- data:
		case <-s.ctx.Done():
			return
		}
	}
}

// handleWebSocketWrites reads from PTY output and sends to WebSocket
func (s *TTYExecSession) handleWebSocketWrites(client *ttyWSClient) {
	for {
		select {
		case data := <-client.send:
			client.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := client.conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
				s.logger.Debug("WebSocket write error", "clientId", client.id, "error", err)
				return
			}
		case <-s.ctx.Done():
			return
		}
	}
}

// ptyReadLoop reads from PTY and broadcasts to all WebSocket clients
func (s *TTYExecSession) ptyReadLoop() {
	defer s.ptmx.Close()

	buffer := make([]byte, 4096)
	for {
		n, err := s.ptmx.Read(buffer)
		if err != nil {
			if err != io.EOF && !errors.Is(err, os.ErrClosed) {
				s.logger.Debug("PTY read error", "error", err)
			}
			return
		}

		if n > 0 {
			data := buffer[:n]
			s.clientsMu.RLock()
			for _, client := range s.clients {
				select {
				case client.send <- data:
				case <-s.ctx.Done():
					s.clientsMu.RUnlock()
					return
				}
			}
			s.clientsMu.RUnlock()
		}
	}
}

// inputWriteLoop reads from input channel and writes to PTY
func (s *TTYExecSession) inputWriteLoop() {
	for {
		select {
		case data := <-s.inCh:
			s.mu.Lock()
			if s.ptmx == nil {
				s.mu.Unlock()
				return
			}
			ptmx := s.ptmx
			s.mu.Unlock()

			_, err := ptmx.Write(data)
			if err != nil {
				if errors.Is(err, os.ErrClosed) {
					return
				}
				s.logger.Debug("PTY write error", "error", err)
				return
			}
		case <-s.ctx.Done():
			return
		}
	}
}

// closeClientsWithExitCode sends exit code to all clients and closes connections
func (s *TTYExecSession) closeClientsWithExitCode(exitCode int, exitReason string) {
	s.clientsMu.RLock()
	clients := make([]*ttyWSClient, 0, len(s.clients))
	for _, client := range s.clients {
		clients = append(clients, client)
	}
	s.clientsMu.RUnlock()

	exitMsg := map[string]interface{}{
		"type":     "control",
		"status":   "exited",
		"exitCode": exitCode,
		"reason":   exitReason,
	}

	for _, client := range clients {
		if b, err := json.Marshal(exitMsg); err == nil {
			client.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			_ = client.conn.WriteMessage(websocket.TextMessage, b)
		}
		_ = client.conn.Close()
	}
}

// ResizeExecuteTTY godoc
//
//	@Summary		Resize TTY execution session
//	@Description	Resize the terminal dimensions of a TTY execution session
//	@Tags			process
//	@Accept			json
//	@Param			sessionId	path		string	true	"TTY execution session ID"
//	@Param			request		body		map		true	"Resize request with cols and rows"
//	@Success		200
//	@Router			/process/execute-tty/{sessionId}/resize [post]
//
//	@id				ResizeExecuteTTY
func ResizeExecuteTTY(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := c.Param("sessionId")
		if sessionID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "session ID is required"})
			return
		}

		var req map[string]uint16
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		cols, okCols := req["cols"]
		rows, okRows := req["rows"]
		if !okCols || !okRows {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cols and rows are required"})
			return
		}

		sessionVal, ok := ttyExecSessions.Load(sessionID)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "TTY execution session not found"})
			return
		}
		session := sessionVal.(*TTYExecSession)

		session.mu.Lock()
		if session.ptmx != nil && session.cmd != nil {
			if err := pty.Setsize(session.ptmx, &pty.Winsize{Cols: cols, Rows: rows}); err != nil {
				session.mu.Unlock()
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resize PTY"})
				return
			}
		}
		session.mu.Unlock()

		c.JSON(http.StatusOK, gin.H{"message": "resized"})
	}
}
