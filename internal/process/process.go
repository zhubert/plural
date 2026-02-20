// Package process provides utilities for managing and cleaning up Claude CLI processes.
package process

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/zhubert/plural-core/logger"
)

// ContainersSupported returns true if Docker is installed on the system.
func ContainersSupported() bool {
	return ContainerCLIInstalled()
}

// ContainerCLIInstalled returns true if the `docker` CLI is on the PATH.
func ContainerCLIInstalled() bool {
	_, err := exec.LookPath("docker")
	return err == nil
}

// containerCheckTimeout is the maximum time to wait for container CLI commands.
const containerCheckTimeout = 5 * time.Second

// ContainerSystemRunning returns true if the Docker daemon is active.
// Returns false if the CLI is not installed, the daemon is not running, or the
// check times out (5s deadline to avoid blocking the UI).
func ContainerSystemRunning() bool {
	if !ContainerCLIInstalled() {
		return false
	}
	return containerSystemRunning()
}

// ContainerImageExists checks if a container image exists locally.
// Returns false if the container CLI is not available or the image is not found.
func ContainerImageExists(image string) bool {
	if !ContainerCLIInstalled() {
		return false
	}
	return containerImageExists(image)
}

// containerSystemRunning checks if the Docker daemon is running.
// Caller must verify CLI is installed first.
func containerSystemRunning() bool {
	ctx, cancel := context.WithTimeout(context.Background(), containerCheckTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "info")
	return cmd.Run() == nil
}

// containerImageExists checks if a container image exists locally.
// Caller must verify CLI is installed first.
func containerImageExists(image string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), containerCheckTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "image", "inspect", image)
	return cmd.Run() == nil
}

// imageUpdateCheckTimeout is the maximum time to wait for remote registry checks.
const imageUpdateCheckTimeout = 15 * time.Second

// CheckContainerImageUpdate checks if a newer version of the container image is
// available in the remote registry. Returns (true, nil) if an update is available,
// (false, nil) if the image is up to date, or (false, err) on failure.
// Requires the Docker CLI to be installed and the image to exist locally.
func CheckContainerImageUpdate(image string) (bool, error) {
	log := logger.WithComponent("process")

	if !ContainerCLIInstalled() {
		return false, fmt.Errorf("docker CLI not installed")
	}

	if !containerImageExists(image) {
		return false, fmt.Errorf("image %s not found locally", image)
	}

	ctx, cancel := context.WithTimeout(context.Background(), imageUpdateCheckTimeout)
	defer cancel()

	// Get local image digest from RepoDigests
	localDigest, err := getLocalImageDigest(ctx, image)
	if err != nil {
		log.Debug("failed to get local image digest", "image", image, "error", err)
		return false, fmt.Errorf("failed to get local digest: %w", err)
	}

	// Get remote manifest digest
	remoteDigest, err := getRemoteManifestDigest(ctx, image)
	if err != nil {
		log.Debug("failed to get remote manifest digest", "image", image, "error", err)
		return false, fmt.Errorf("failed to get remote digest: %w", err)
	}

	needsUpdate := localDigest != remoteDigest
	if needsUpdate {
		log.Info("container image update available", "image", image, "local", localDigest, "remote", remoteDigest)
	} else {
		log.Debug("container image is up to date", "image", image)
	}

	return needsUpdate, nil
}

// imageInspect represents the relevant fields from docker image inspect JSON output.
type imageInspect struct {
	RepoDigests []string `json:"RepoDigests"`
}

// getLocalImageDigest returns the sha256 digest from the image's RepoDigests.
// Returns an error if the image has no RepoDigests (e.g., locally built images
// that were never pushed/pulled from a registry).
func getLocalImageDigest(ctx context.Context, image string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", "image", "inspect", image, "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	// docker image inspect returns a JSON array
	var inspects []imageInspect
	if err := json.Unmarshal(output, &inspects); err != nil {
		return "", fmt.Errorf("failed to parse image inspect output: %w", err)
	}

	if len(inspects) == 0 || len(inspects[0].RepoDigests) == 0 {
		return "", fmt.Errorf("image has no repo digests (locally built?)")
	}

	// RepoDigests format: "ghcr.io/user/image@sha256:abc123..."
	repoDigest := inspects[0].RepoDigests[0]
	if _, after, ok := strings.Cut(repoDigest, "@"); ok {
		return after, nil
	}

	return "", fmt.Errorf("no digest found in RepoDigests: %s", repoDigest)
}

// getRemoteManifestDigest queries the container registry API to get the manifest
// digest for the given image tag. It returns the Docker-Content-Digest header,
// which for multi-platform images is the manifest list digest — matching what
// Docker stores in RepoDigests locally.
//
// The function follows the OCI distribution spec auth flow: try unauthenticated
// first, then parse WWW-Authenticate on 401 to obtain a bearer token.
func getRemoteManifestDigest(ctx context.Context, image string) (string, error) {
	registry, repo, tag := parseImageRef(image)
	manifestURL := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, repo, tag)

	// Accept headers for manifest list and single-platform manifests.
	acceptHeader := strings.Join([]string{
		"application/vnd.oci.image.index.v1+json",
		"application/vnd.docker.distribution.manifest.list.v2+json",
		"application/vnd.oci.image.manifest.v1+json",
		"application/vnd.docker.distribution.manifest.v2+json",
	}, ", ")

	client := &http.Client{Timeout: imageUpdateCheckTimeout}

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, manifestURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", acceptHeader)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("registry request failed: %w", err)
	}
	defer resp.Body.Close()

	// If unauthorized, try to get a bearer token and retry.
	if resp.StatusCode == http.StatusUnauthorized {
		token, tokenErr := getRegistryToken(ctx, client, resp.Header.Get("Www-Authenticate"), repo)
		if tokenErr != nil {
			return "", fmt.Errorf("failed to get registry token: %w", tokenErr)
		}

		req, err = http.NewRequestWithContext(ctx, http.MethodHead, manifestURL, nil)
		if err != nil {
			return "", fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Accept", acceptHeader)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err = client.Do(req)
		if err != nil {
			return "", fmt.Errorf("registry request failed: %w", err)
		}
		defer resp.Body.Close()
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("registry returned status %d", resp.StatusCode)
	}

	digest := resp.Header.Get("Docker-Content-Digest")
	if digest == "" {
		return "", fmt.Errorf("no Docker-Content-Digest header in response")
	}

	return digest, nil
}

// parseImageRef splits a Docker image reference into registry, repository, and tag.
// Examples:
//
//	"ghcr.io/user/image:v1"   → ("ghcr.io", "user/image", "v1")
//	"ghcr.io/user/image"      → ("ghcr.io", "user/image", "latest")
//	"ubuntu:22.04"             → ("registry-1.docker.io", "library/ubuntu", "22.04")
//	"ubuntu"                   → ("registry-1.docker.io", "library/ubuntu", "latest")
func parseImageRef(image string) (registry, repo, tag string) {
	tag = "latest"

	// Split off tag (last colon after last slash)
	if lastSlash := strings.LastIndex(image, "/"); lastSlash != -1 {
		if colonIdx := strings.LastIndex(image[lastSlash:], ":"); colonIdx != -1 {
			tag = image[lastSlash+colonIdx+1:]
			image = image[:lastSlash+colonIdx]
		}
	} else if colonIdx := strings.LastIndex(image, ":"); colonIdx != -1 {
		tag = image[colonIdx+1:]
		image = image[:colonIdx]
	}

	// Determine if the first component is a registry (contains "." or ":")
	parts := strings.SplitN(image, "/", 2)
	if len(parts) == 1 {
		// No slash: Docker Hub library image (e.g., "ubuntu")
		return "registry-1.docker.io", "library/" + parts[0], tag
	}

	firstPart := parts[0]
	if strings.Contains(firstPart, ".") || strings.Contains(firstPart, ":") {
		// First part looks like a registry hostname
		return firstPart, parts[1], tag
	}

	// No registry prefix: Docker Hub user image (e.g., "user/image")
	return "registry-1.docker.io", image, tag
}

// tokenResponse represents the JSON response from a registry token endpoint.
type tokenResponse struct {
	Token       string `json:"token"`
	AccessToken string `json:"access_token"`
}

// getRegistryToken parses a WWW-Authenticate header and fetches a bearer token.
// The header format is: Bearer realm="<url>",service="<svc>",scope="<scope>"
func getRegistryToken(ctx context.Context, client *http.Client, wwwAuth, repo string) (string, error) {
	realm, params := parseWWWAuthenticate(wwwAuth)
	if realm == "" {
		return "", fmt.Errorf("no realm in WWW-Authenticate header: %s", wwwAuth)
	}

	// Build token request URL with query parameters
	tokenReq, err := http.NewRequestWithContext(ctx, http.MethodGet, realm, nil)
	if err != nil {
		return "", err
	}

	q := tokenReq.URL.Query()
	if svc, ok := params["service"]; ok {
		q.Set("service", svc)
	}
	if scope, ok := params["scope"]; ok {
		q.Set("scope", scope)
	} else {
		q.Set("scope", "repository:"+repo+":pull")
	}
	tokenReq.URL.RawQuery = q.Encode()

	resp, err := client.Do(tokenReq)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token endpoint returned status %d", resp.StatusCode)
	}

	var tr tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", fmt.Errorf("failed to parse token response: %w", err)
	}

	if tr.Token != "" {
		return tr.Token, nil
	}
	if tr.AccessToken != "" {
		return tr.AccessToken, nil
	}
	return "", fmt.Errorf("no token in response")
}

// parseWWWAuthenticate parses a Bearer WWW-Authenticate header into realm and key-value params.
// Input:  `Bearer realm="https://ghcr.io/token",service="ghcr.io",scope="repository:user/repo:pull"`
// Output: ("https://ghcr.io/token", {"service":"ghcr.io","scope":"repository:user/repo:pull"})
func parseWWWAuthenticate(header string) (realm string, params map[string]string) {
	params = make(map[string]string)

	header = strings.TrimPrefix(header, "Bearer ")
	header = strings.TrimPrefix(header, "bearer ")

	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		value = strings.Trim(value, `"`)
		if key == "realm" {
			realm = value
		} else {
			params[key] = value
		}
	}
	return realm, params
}

// ContainerPrerequisites holds the results of all container prerequisite checks.
type ContainerPrerequisites struct {
	CLIInstalled  bool
	SystemRunning bool
	ImageExists   bool
	AuthAvailable bool
}

// CheckContainerPrerequisites runs all container prerequisite checks with short-circuiting.
// Later checks are skipped when earlier ones fail, since they depend on the previous step.
// authChecker is a function that returns whether auth credentials are available.
func CheckContainerPrerequisites(image string, authChecker func() bool) ContainerPrerequisites {
	result := ContainerPrerequisites{}

	result.CLIInstalled = ContainerCLIInstalled()
	if !result.CLIInstalled {
		return result
	}

	result.SystemRunning = containerSystemRunning()
	if !result.SystemRunning {
		return result
	}

	result.ImageExists = containerImageExists(image)
	if !result.ImageExists {
		return result
	}

	result.AuthAvailable = authChecker()
	return result
}

// ClaudeProcess represents a running Claude CLI process found on the system.
type ClaudeProcess struct {
	PID     int    // Process ID
	Command string // Full command line
}

// FindClaudeProcesses finds all running Claude CLI processes on the system.
// This is useful for detecting orphaned processes that may have been left behind
// after a crash.
func FindClaudeProcesses() ([]ClaudeProcess, error) {
	var processes []ClaudeProcess
	log := logger.WithComponent("process")

	switch runtime.GOOS {
	case "darwin", "linux":
		// Use pgrep to find claude processes
		cmd := exec.Command("pgrep", "-f", "claude.*--session-id")
		output, err := cmd.Output()
		if err != nil {
			// pgrep returns exit code 1 if no processes found
			if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
				return processes, nil
			}
			return nil, err
		}

		pids := strings.Fields(string(output))
		for _, pidStr := range pids {
			pid, err := strconv.Atoi(strings.TrimSpace(pidStr))
			if err != nil {
				continue
			}

			// Get the full command line for this PID
			psCmd := exec.Command("ps", "-p", pidStr, "-o", "args=")
			psOutput, err := psCmd.Output()
			if err != nil {
				continue
			}

			processes = append(processes, ClaudeProcess{
				PID:     pid,
				Command: strings.TrimSpace(string(psOutput)),
			})
		}

	case "windows":
		// Use tasklist on Windows
		cmd := exec.Command("tasklist", "/FI", "IMAGENAME eq claude*", "/FO", "CSV", "/NH")
		output, err := cmd.Output()
		if err != nil {
			return nil, err
		}

		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			fields := strings.Split(line, ",")
			if len(fields) >= 2 {
				// Remove quotes from PID field
				pidStr := strings.Trim(strings.TrimSpace(fields[1]), "\"")
				pid, err := strconv.Atoi(pidStr)
				if err != nil {
					continue
				}
				processes = append(processes, ClaudeProcess{
					PID:     pid,
					Command: strings.Trim(fields[0], "\""),
				})
			}
		}
	}

	log.Debug("found Claude processes", "count", len(processes))
	return processes, nil
}

// KillProcess kills a process by PID.
func KillProcess(pid int) error {
	switch runtime.GOOS {
	case "darwin", "linux":
		cmd := exec.Command("kill", "-9", strconv.Itoa(pid))
		return cmd.Run()
	case "windows":
		cmd := exec.Command("taskkill", "/F", "/PID", strconv.Itoa(pid))
		return cmd.Run()
	}
	return nil
}

// FindOrphanedClaudeProcesses finds Claude processes that have specific session IDs
// that aren't in the provided list of known session IDs.
func FindOrphanedClaudeProcesses(knownSessionIDs map[string]bool) ([]ClaudeProcess, error) {
	allProcesses, err := FindClaudeProcesses()
	if err != nil {
		return nil, err
	}

	log := logger.WithComponent("process")
	var orphans []ClaudeProcess
	for _, proc := range allProcesses {
		sessionID := extractSessionID(proc.Command)
		if sessionID != "" && !knownSessionIDs[sessionID] {
			orphans = append(orphans, proc)
			log.Info("found orphaned Claude process", "pid", proc.PID, "sessionID", sessionID)
		}
	}

	return orphans, nil
}

// extractSessionID extracts the session ID from a Claude CLI command line.
func extractSessionID(cmdLine string) string {
	// Look for --session-id or --resume followed by the ID
	patterns := []string{"--session-id", "--resume"}
	for _, pattern := range patterns {
		_, after, ok := strings.Cut(cmdLine, pattern)
		if !ok {
			continue
		}

		// Get the part after the flag
		rest := after
		rest = strings.TrimLeft(rest, " =")

		// Extract the session ID (first space-separated token)
		fields := strings.Fields(rest)
		if len(fields) > 0 {
			return fields[0]
		}
	}
	return ""
}

// CleanupOrphanedProcesses kills all Claude processes that don't match known session IDs.
// Returns the number of processes killed.
func CleanupOrphanedProcesses(knownSessionIDs map[string]bool) (int, error) {
	orphans, err := FindOrphanedClaudeProcesses(knownSessionIDs)
	if err != nil {
		return 0, err
	}

	log := logger.WithComponent("process")
	killed := 0
	for _, proc := range orphans {
		log.Info("killing orphaned Claude process", "pid", proc.PID)
		if err := KillProcess(proc.PID); err != nil {
			log.Error("failed to kill process", "pid", proc.PID, "error", err)
			continue
		}
		killed++
	}

	return killed, nil
}

// OrphanedContainer represents a container found on the system that doesn't match any known session.
type OrphanedContainer struct {
	Name string // Container name (e.g., "plural-abc123")
}

// ListContainerNames returns a list of all container names using the Docker CLI.
// Docker outputs NDJSON (one JSON object per line) with a "Names" field.
func ListContainerNames() ([]string, error) {
	cmd := exec.Command("docker", "ps", "-a", "--format", "{{.Names}}")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var names []string
	for line := range strings.SplitSeq(strings.TrimSpace(string(output)), "\n") {
		name := strings.TrimSpace(line)
		if name != "" {
			names = append(names, name)
		}
	}

	return names, nil
}

// FindOrphanedContainers finds containers named plural-* whose session ID is not in knownSessionIDs.
// Returns an empty list (not an error) if the container CLI is not available.
func FindOrphanedContainers(knownSessionIDs map[string]bool) ([]OrphanedContainer, error) {
	log := logger.WithComponent("process")

	// Check if container CLI is available
	if _, err := exec.LookPath("docker"); err != nil {
		log.Debug("docker CLI not found, skipping container orphan check")
		return nil, nil
	}

	// Get list of container names
	names, err := ListContainerNames()
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	var orphans []OrphanedContainer
	for _, name := range names {
		if name == "" {
			continue
		}
		if !strings.HasPrefix(name, "plural-") {
			continue
		}
		sessionID := strings.TrimPrefix(name, "plural-")
		if !knownSessionIDs[sessionID] {
			orphans = append(orphans, OrphanedContainer{Name: name})
			log.Info("found orphaned container", "name", name, "sessionID", sessionID)
		}
	}

	log.Debug("found orphaned containers", "count", len(orphans))
	return orphans, nil
}

// CleanupOrphanedContainers removes all containers named plural-* that don't match known session IDs.
// Returns the number of containers removed.
func CleanupOrphanedContainers(knownSessionIDs map[string]bool) (int, error) {
	orphans, err := FindOrphanedContainers(knownSessionIDs)
	if err != nil {
		return 0, err
	}

	log := logger.WithComponent("process")
	removed := 0
	for _, container := range orphans {
		log.Info("removing orphaned container", "name", container.Name)
		cmd := exec.Command("docker", "rm", "-f", container.Name)
		if err := cmd.Run(); err != nil {
			log.Error("failed to remove container", "name", container.Name, "error", err)
			continue
		}
		removed++
	}

	return removed, nil
}
