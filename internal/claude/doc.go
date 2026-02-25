// Package claude provides a wrapper around the Claude Code CLI for managing
// conversation sessions.
//
// # Overview
//
// The claude package manages Claude Code CLI processes, handling message
// streaming, session persistence, and permission prompts. Each session
// maintains its own Runner instance which tracks conversation state.
//
// # Runner
//
// Runner is the main type that manages a Claude Code CLI session:
//
//	runner := claude.New(sessionID, workingDir, repoPath, sessionStarted, initialMessages)
//	responseChan := runner.Send(ctx, "Hello, Claude!")
//	for chunk := range responseChan {
//	    if chunk.Error != nil {
//	        // Handle error
//	    }
//	    fmt.Print(chunk.Content)
//	}
//
// # Session Management
//
// Sessions are tracked using:
//   - sessionID: Unique identifier passed to Claude CLI
//   - workingDir: The git worktree directory where Claude operates
//   - sessionStarted: Whether this session has been started before
//
// First messages use --session-id flag, subsequent messages use --resume flag.
// This allows Claude to maintain conversation context across multiple prompts.
//
// # Message Streaming
//
// Responses from Claude are streamed through a channel:
//
//	type ResponseChunk struct {
//	    Content string  // Text content
//	    Done    bool    // True when response is complete
//	    Error   error   // Non-nil if an error occurred
//	}
//
// The Send() method returns immediately with a channel. Content chunks are
// sent as they arrive, with a final chunk having Done=true.
//
// # Permission System
//
// Claude requires permissions for operations like file edits and bash commands.
// The Runner integrates with the MCP permission system:
//
//	runner.PermissionRequestChan()  // Receives permission requests
//	runner.SendPermissionResponse() // Sends user's decision
//	runner.AddAllowedTool()         // Pre-allow specific tools
//
// When Claude needs permission:
//  1. An MCP server subprocess is started with the Runner
//  2. Permission requests come through PermissionRequestChan()
//  3. The TUI shows a permission modal
//  4. User's decision is sent via SendPermissionResponse()
//  5. "Always Allow" decisions are persisted for future use
//
// # Constants
//
//   - PermissionChannelBuffer: Buffer size for permission channels (1)
//   - PermissionTimeout: Maximum wait time for permission responses (5 minutes)
//
// # Thread Safety
//
// Runner is thread-safe. All operations that modify state are protected
// by a sync.RWMutex. Multiple goroutines can safely call methods like
// GetMessages() while a conversation is in progress.
//
// # Resource Cleanup
//
// Call Stop() when done with a Runner to clean up resources:
//
//	runner.Stop()
//
// This closes the permission channels and releases any pending goroutines.
package claude
