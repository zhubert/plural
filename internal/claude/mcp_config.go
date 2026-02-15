package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/zhubert/plural/internal/mcp"
)

// ContainerGatewayIP is the hostname of the host as seen from inside a Docker
// container. Docker Desktop provides this automatically; on Linux Docker Engine,
// the --add-host flag maps it to the host gateway.
const ContainerGatewayIP = "host.docker.internal"

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
// Unix sockets can't reliably cross the Docker container boundary. The MCP subprocess
// inside the container connects back to the host via TCP using host.docker.internal.
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

	// Build optional socket server options for supervisor channels
	var socketOpts []mcp.SocketServerOption
	if r.supervisor && r.mcp.CreateChildReq != nil {
		socketOpts = append(socketOpts, mcp.WithSupervisorChannels(
			r.mcp.CreateChildReq, r.mcp.CreateChildResp,
			r.mcp.ListChildrenReq, r.mcp.ListChildrenResp,
			r.mcp.MergeChildReq, r.mcp.MergeChildResp,
		))
	}

	// Build optional socket server options for host tool channels
	if r.hostTools && r.mcp.CreatePRReq != nil {
		socketOpts = append(socketOpts, mcp.WithHostToolChannels(
			r.mcp.CreatePRReq, r.mcp.CreatePRResp,
			r.mcp.PushBranchReq, r.mcp.PushBranchResp,
			r.mcp.GetReviewCommentsReq, r.mcp.GetReviewCommentsResp,
		))
	}

	if r.containerized {
		// Container sessions use TCP because Unix sockets don't work across
		// the Docker container boundary.
		socketServer, err = mcp.NewTCPSocketServer(r.sessionID,
			r.mcp.PermissionReq, r.mcp.PermissionResp,
			r.mcp.QuestionReq, r.mcp.QuestionResp,
			r.mcp.PlanReq, r.mcp.PlanResp, socketOpts...)
	} else {
		socketServer, err = mcp.NewSocketServer(r.sessionID,
			r.mcp.PermissionReq, r.mcp.PermissionResp,
			r.mcp.QuestionReq, r.mcp.QuestionResp,
			r.mcp.PlanReq, r.mcp.PlanResp, socketOpts...)
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
	mcpArgs := []string{"mcp-server", "--socket", socketPath}
	if r.supervisor {
		mcpArgs = append(mcpArgs, "--supervisor")
	}
	if r.hostTools {
		mcpArgs = append(mcpArgs, "--host-tools")
	}
	mcpServers := map[string]interface{}{
		"plural": map[string]interface{}{
			"command": execPath,
			"args":    mcpArgs,
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
	if err := os.WriteFile(configPath, configJSON, 0600); err != nil {
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
	args := []string{"mcp-server", "--tcp", tcpAddr, "--auto-approve", "--session-id", r.sessionID}
	if r.supervisor {
		args = append(args, "--supervisor")
	}
	if r.hostTools {
		args = append(args, "--host-tools")
	}
	mcpServers := map[string]interface{}{
		"plural": map[string]interface{}{
			"command": "/usr/local/bin/plural",
			"args":    args,
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
	if err := os.WriteFile(configPath, configJSON, 0600); err != nil {
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
