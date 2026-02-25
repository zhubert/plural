// Package mcp implements the Model Context Protocol (MCP) for handling permission prompts.
//
// # Overview
//
// The MCP package provides a mechanism for Claude Code CLI to request permissions
// from the user through the Plural TUI. This is done through a three-layer architecture:
//
//  1. MCP Server (subprocess): Runs as a separate process with stdin/stdout communication.
//     Implements the JSON-RPC 2.0 protocol and receives tool calls from Claude CLI.
//
//  2. Unix Socket IPC: Communication channel between the MCP server subprocess and the TUI.
//     Located at /tmp/plural-<session-id>.sock. The MCP server sends permission requests
//     through this socket and receives responses.
//
//  3. TUI Permission Modal: Displays the permission request to the user and captures
//     their decision (Allow, Deny, or Always Allow).
//
// # Permission Flow
//
// The permission flow works as follows:
//
//	Claude CLI
//	    ↓ (--permission-prompt-tool mcp__plural__permission)
//	MCP Server (subprocess)
//	    ↓ (JSON-RPC tool call)
//	SocketServer.Run()
//	    ↓ (Unix socket)
//	TUI Modal.Show() ← permission request displayed
//	    ↓
//	User presses y/n/a
//	    ↓
//	Modal hides, response sent via socket
//	    ↓
//	MCP Server sends tool result back to Claude CLI
//
// # Components
//
// Server: The main MCP server implementation that handles JSON-RPC 2.0 messages
// over stdin/stdout. It processes tools/list and tools/call requests.
//
// SocketServer: Listens for permission requests from MCP server subprocesses
// and forwards them to the TUI. Manages the Unix socket lifecycle.
//
// SocketClient: Used by the MCP server subprocess to connect to the TUI's
// socket server and send permission requests.
//
// # Timeouts
//
// To prevent deadlocks, all socket operations have timeouts:
//   - PermissionResponseTimeout: 5 minutes for user to respond to permission prompt
//   - SocketReadTimeout: 10 seconds for socket read operations
//
// If a timeout occurs, the permission is automatically denied.
package mcp
