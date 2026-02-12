package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/zhubert/plural/internal/mcp"
)

// ContainerGatewayIP is the IP address of the host as seen from inside an Apple
// container. This is the default gateway in the container's network namespace.
const ContainerGatewayIP = "192.168.64.1"

// MCPServer represents an external MCP server configuration
type MCPServer struct {
	Name    string
	Command string
	Args    []string
}

// ensureServerRunning starts the socket server and creates MCP config if not already running.
// This makes the MCP server persistent across multiple Send() calls within a session.
//
// For containerized sessions, the server listens on TCP instead of a Unix socket because
// Unix sockets can't reliably cross the Apple container boundary. The MCP subprocess
// inside the container connects back to the host via TCP using the container gateway IP.
func (r *Runner) ensureServerRunning() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.serverRunning {
		return nil
	}

	r.log.Info("starting persistent MCP server")
	startTime := time.Now()

	var socketServer *mcp.SocketServer
	var err error

	if r.containerized {
		// Container sessions use TCP because Unix sockets don't work across
		// Apple's container boundary (vsock proxy doesn't support client→host connections).
		socketServer, err = mcp.NewTCPSocketServer(r.sessionID,
			r.mcp.PermissionReq, r.mcp.PermissionResp,
			r.mcp.QuestionReq, r.mcp.QuestionResp,
			r.mcp.PlanReq, r.mcp.PlanResp)
	} else {
		socketServer, err = mcp.NewSocketServer(r.sessionID,
			r.mcp.PermissionReq, r.mcp.PermissionResp,
			r.mcp.QuestionReq, r.mcp.QuestionResp,
			r.mcp.PlanReq, r.mcp.PlanResp)
	}
	if err != nil {
		r.log.Error("failed to create socket server", "error", err)
		return fmt.Errorf("failed to start permission server: %v", err)
	}
	r.socketServer = socketServer
	r.log.Debug("socket server created", "elapsed", time.Since(startTime))

	// Start socket server in background (Start() calls wg.Add before launching goroutine)
	r.socketServer.Start()

	// Create MCP config file — different config for containerized vs host sessions.
	// Container config uses TCP address; host config uses Unix socket path.
	var mcpConfigPath string
	if r.containerized {
		tcpAddr := fmt.Sprintf("%s:%d", ContainerGatewayIP, r.socketServer.TCPPort())
		mcpConfigPath, err = r.createContainerMCPConfigLocked(tcpAddr)
	} else {
		mcpConfigPath, err = r.createMCPConfigLocked(r.socketServer.SocketPath())
	}
	if err != nil {
		r.socketServer.Close()
		r.socketServer = nil
		r.log.Error("failed to create MCP config", "error", err)
		return fmt.Errorf("failed to create MCP config: %v", err)
	}
	r.mcpConfigPath = mcpConfigPath

	r.serverRunning = true
	if r.containerized {
		r.log.Info("persistent MCP server started (TCP)",
			"elapsed", time.Since(startTime),
			"tcpAddr", r.socketServer.TCPAddr(),
			"config", r.mcpConfigPath)
	} else {
		r.log.Info("persistent MCP server started",
			"elapsed", time.Since(startTime),
			"socket", r.socketServer.SocketPath(),
			"config", r.mcpConfigPath)
	}

	return nil
}

// createMCPConfigLocked creates the MCP config file. Must be called with mu held.
func (r *Runner) createMCPConfigLocked(socketPath string) (string, error) {
	execPath, err := os.Executable()
	if err != nil {
		return "", err
	}

	// Start with the plural permission handler
	mcpServers := map[string]interface{}{
		"plural": map[string]interface{}{
			"command": execPath,
			"args":    []string{"mcp-server", "--socket", socketPath},
		},
	}

	// Add external MCP servers
	for _, server := range r.mcpServers {
		mcpServers[server.Name] = map[string]interface{}{
			"command": server.Command,
			"args":    server.Args,
		}
	}

	config := map[string]interface{}{
		"mcpServers": mcpServers,
	}

	configJSON, err := json.Marshal(config)
	if err != nil {
		return "", err
	}

	configPath := filepath.Join(os.TempDir(), fmt.Sprintf("plural-mcp-%s.json", r.sessionID))
	if err := os.WriteFile(configPath, configJSON, 0644); err != nil {
		return "", err
	}

	return configPath, nil
}

// createContainerMCPConfigLocked creates the MCP config for containerized sessions.
// The config points to the plural binary inside the container at /usr/local/bin/plural
// with --auto-approve and --tcp, which auto-approves all regular permissions while
// routing AskUserQuestion and ExitPlanMode through the TUI via TCP.
// Must be called with mu held.
func (r *Runner) createContainerMCPConfigLocked(tcpAddr string) (string, error) {
	mcpServers := map[string]interface{}{
		"plural": map[string]interface{}{
			"command": "/usr/local/bin/plural",
			"args":    []string{"mcp-server", "--tcp", tcpAddr, "--auto-approve", "--session-id", r.sessionID},
		},
	}

	config := map[string]interface{}{
		"mcpServers": mcpServers,
	}

	configJSON, err := json.Marshal(config)
	if err != nil {
		return "", err
	}

	configPath := filepath.Join(os.TempDir(), fmt.Sprintf("plural-mcp-%s.json", r.sessionID))
	if err := os.WriteFile(configPath, configJSON, 0644); err != nil {
		return "", err
	}

	return configPath, nil
}

// SetMCPServers sets the external MCP servers to include in the config
func (r *Runner) SetMCPServers(servers []MCPServer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mcpServers = servers
	r.log.Debug("set external MCP servers", "count", len(servers))
}
