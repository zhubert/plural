package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/zhubert/plural/internal/mcp"
	"github.com/zhubert/plural/internal/paths"
)

// MCPServer represents an external MCP server configuration
type MCPServer struct {
	Name    string
	Command string
	Args    []string
}

// ensureServerRunning starts the socket server and creates MCP config if not already running.
// This makes the MCP server persistent across multiple Send() calls within a session.
//
// For containerized sessions, the TCP direction is reversed: the MCP subprocess inside the
// container listens on a port, Docker publishes it, and the host dials in. This avoids
// macOS firewall issues that block inbound connections to the host.
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
	if r.supervisor && r.mcp.CreateChild != nil {
		socketOpts = append(socketOpts, mcp.WithSupervisorChannels(
			r.mcp.CreateChild.Req, r.mcp.CreateChild.Resp,
			r.mcp.ListChildren.Req, r.mcp.ListChildren.Resp,
			r.mcp.MergeChild.Req, r.mcp.MergeChild.Resp,
		))
	}

	// Build optional socket server options for host tool channels
	if r.hostTools && r.mcp.CreatePR != nil {
		socketOpts = append(socketOpts, mcp.WithHostToolChannels(
			r.mcp.CreatePR.Req, r.mcp.CreatePR.Resp,
			r.mcp.PushBranch.Req, r.mcp.PushBranch.Resp,
			r.mcp.GetReviewComments.Req, r.mcp.GetReviewComments.Resp,
		))
	}

	if r.containerized {
		// Container sessions use a dialing server: the MCP subprocess inside the
		// container listens on a port, and the host dials in. This reverses the TCP
		// direction so that macOS firewall rules don't block the connection.
		socketServer = mcp.NewDialingSocketServer(r.sessionID,
			r.mcp.Permission.Req, r.mcp.Permission.Resp,
			r.mcp.Question.Req, r.mcp.Question.Resp,
			r.mcp.PlanApproval.Req, r.mcp.PlanApproval.Resp, socketOpts...)
	} else {
		socketServer, err = mcp.NewSocketServer(r.sessionID,
			r.mcp.Permission.Req, r.mcp.Permission.Resp,
			r.mcp.Question.Req, r.mcp.Question.Resp,
			r.mcp.PlanApproval.Req, r.mcp.PlanApproval.Resp, socketOpts...)
	}
	if err != nil {
		r.log.Error("failed to create socket server", "error", err)
		return fmt.Errorf("failed to start permission server: %v", err)
	}
	r.socketServer = socketServer
	r.log.Debug("socket server created", "elapsed", time.Since(startTime))

	// Start socket server in background for non-container sessions.
	// Container sessions use a dialing server with no accept loop — the host
	// dials into the container and passes the connection via HandleConn().
	if !r.containerized {
		r.socketServer.Start()
	}

	// Create MCP config file — different config for containerized vs host sessions.
	// Container config uses --listen with a fixed port; host config uses Unix socket path.
	var mcpConfigPath string
	if r.containerized {
		mcpConfigPath, err = r.createContainerMCPConfigLocked(mcp.ContainerMCPPort)
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
		r.log.Info("persistent MCP server started (dialing, reverse TCP)",
			"elapsed", time.Since(startTime),
			"containerPort", mcp.ContainerMCPPort,
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
	mcpServers := map[string]any{
		"plural": map[string]any{
			"command": execPath,
			"args":    mcpArgs,
		},
	}

	// Add external MCP servers
	for _, server := range r.mcpServers {
		mcpServers[server.Name] = map[string]any{
			"command": server.Command,
			"args":    server.Args,
		}
	}

	config := map[string]any{
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
// with --auto-approve and --listen, which auto-approves all regular permissions while
// routing AskUserQuestion and ExitPlanMode through the TUI via reverse TCP.
// The MCP subprocess listens on containerPort and the host dials in.
// Must be called with mu held.
func (r *Runner) createContainerMCPConfigLocked(containerPort int) (string, error) {
	listenAddr := fmt.Sprintf("0.0.0.0:%d", containerPort)
	args := []string{"mcp-server", "--listen", listenAddr, "--auto-approve", "--session-id", r.sessionID}
	if r.supervisor {
		args = append(args, "--supervisor")
	}
	if r.hostTools {
		args = append(args, "--host-tools")
	}
	mcpServers := map[string]any{
		"plural": map[string]any{
			"command": "/usr/local/bin/plural",
			"args":    args,
		},
	}

	config := map[string]any{
		"mcpServers": mcpServers,
	}

	configJSON, err := json.Marshal(config)
	if err != nil {
		return "", err
	}

	// Write to ~/.plural/ (under $HOME) instead of os.TempDir() (/var/folders/ on macOS).
	// Colima shares $HOME with the Docker VM by default, but /var/folders/ is NOT shared.
	// When Docker can't access the host file, it creates an empty directory at the mount
	// point inside the container, causing Claude CLI to hang trying to read a directory
	// as a JSON file.
	configDir, err := paths.ConfigDir()
	if err != nil {
		// Fall back to temp dir if config dir is unavailable
		configDir = os.TempDir()
	}
	configPath := filepath.Join(configDir, fmt.Sprintf("plural-mcp-%s.json", r.sessionID))
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
