package claude

import (
	"encoding/json"
	"os"
	"testing"
)

func TestCreateContainerMCPConfigLocked(t *testing.T) {
	r := &Runner{
		sessionID: "test-container-mcp",
		log:       pmTestLogger(),
	}

	// Use a container port (what ensureServerRunning passes for container sessions)
	containerPort := 21120
	configPath, err := r.createContainerMCPConfigLocked(containerPort)
	if err != nil {
		t.Fatalf("createContainerMCPConfigLocked() error = %v", err)
	}
	defer os.Remove(configPath)

	// Read and parse the config
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("failed to parse config JSON: %v", err)
	}

	// Verify mcpServers structure
	mcpServers, ok := config["mcpServers"].(map[string]any)
	if !ok {
		t.Fatal("expected mcpServers key in config")
	}

	plural, ok := mcpServers["plural"].(map[string]any)
	if !ok {
		t.Fatal("expected 'plural' server in mcpServers")
	}

	// Verify command points to in-container binary
	command, ok := plural["command"].(string)
	if !ok {
		t.Fatal("expected 'command' field")
	}
	if command != "/usr/local/bin/plural" {
		t.Errorf("command = %q, want '/usr/local/bin/plural'", command)
	}

	// Verify args include --auto-approve and --listen (not --socket or --tcp)
	argsRaw, ok := plural["args"].([]any)
	if !ok {
		t.Fatal("expected 'args' field to be array")
	}

	args := make([]string, len(argsRaw))
	for i, a := range argsRaw {
		args[i], _ = a.(string)
	}

	if !containsArg(args, "--auto-approve") {
		t.Error("container MCP config should include --auto-approve flag")
	}
	if !containsArg(args, "--listen") {
		t.Error("container MCP config should include --listen flag")
	}
	if containsArg(args, "--tcp") {
		t.Error("container MCP config should NOT include --tcp flag (--listen is used instead)")
	}
	if containsArg(args, "--socket") {
		t.Error("container MCP config should NOT include --socket flag")
	}
	if got := getArgValue(args, "--listen"); got != "0.0.0.0:21120" {
		t.Errorf("--listen value = %q, want %q", got, "0.0.0.0:21120")
	}
	if !containsArg(args, "mcp-server") {
		t.Error("container MCP config should include 'mcp-server' subcommand")
	}
	if !containsArg(args, "--session-id") {
		t.Error("container MCP config should include --session-id flag")
	}
	if got := getArgValue(args, "--session-id"); got != "test-container-mcp" {
		t.Errorf("--session-id value = %q, want %q", got, "test-container-mcp")
	}
}

func TestCreateMCPConfigLocked_HostSession(t *testing.T) {
	r := &Runner{
		sessionID: "test-host-mcp",
		log:       pmTestLogger(),
	}

	socketPath := "/tmp/plural-test-host-mcp.sock"
	configPath, err := r.createMCPConfigLocked(socketPath)
	if err != nil {
		t.Fatalf("createMCPConfigLocked() error = %v", err)
	}
	defer os.Remove(configPath)

	// Read and parse the config
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("failed to parse config JSON: %v", err)
	}

	mcpServers, ok := config["mcpServers"].(map[string]any)
	if !ok {
		t.Fatal("expected mcpServers key in config")
	}

	plural, ok := mcpServers["plural"].(map[string]any)
	if !ok {
		t.Fatal("expected 'plural' server in mcpServers")
	}

	// Host config should NOT use /usr/local/bin/plural
	command, ok := plural["command"].(string)
	if !ok {
		t.Fatal("expected 'command' field")
	}
	if command == "/usr/local/bin/plural" {
		t.Error("host MCP config should use the current executable, not /usr/local/bin/plural")
	}

	// Host config should NOT include --auto-approve
	argsRaw, ok := plural["args"].([]any)
	if !ok {
		t.Fatal("expected 'args' field to be array")
	}
	args := make([]string, len(argsRaw))
	for i, a := range argsRaw {
		args[i], _ = a.(string)
	}
	if containsArg(args, "--auto-approve") {
		t.Error("host MCP config should NOT include --auto-approve flag")
	}
}

func TestCreateContainerMCPConfig_NoExternalServers(t *testing.T) {
	// Container MCP config should not include external MCP servers
	// (not supported in container mode)
	r := &Runner{
		sessionID: "test-no-external",
		log:       pmTestLogger(),
		mcpServers: []MCPServer{
			{Name: "external", Command: "/usr/local/bin/external", Args: []string{"serve"}},
		},
	}

	configPath, err := r.createContainerMCPConfigLocked(21120)
	if err != nil {
		t.Fatalf("createContainerMCPConfigLocked() error = %v", err)
	}
	defer os.Remove(configPath)

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("failed to parse config JSON: %v", err)
	}

	mcpServers := config["mcpServers"].(map[string]any)
	if _, exists := mcpServers["external"]; exists {
		t.Error("container MCP config should not include external MCP servers")
	}
}
