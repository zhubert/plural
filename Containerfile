FROM node:22-slim

# Install git (needed for worktree operations inside container)
RUN apt-get update && apt-get install -y --no-install-recommends git && rm -rf /var/lib/apt/lists/*

# Install Claude CLI globally
RUN npm install -g @anthropic-ai/claude-code

# Default working directory (overridden by -w flag)
WORKDIR /workspace

ENTRYPOINT ["claude"]
