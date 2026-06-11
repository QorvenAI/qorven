// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/qorvenai/qorven/internal/gateway/deploy"
)

// localDeployTarget builds a Docker image from a project path and runs it on
// a deterministic localhost port.  The returned URL is always
// http://127.0.0.1:<port> — a real, reachable address for dev/self-hosted use.
type localDeployTarget struct {
	gw *Gateway
}

// newLocalTarget constructs a localDeployTarget.
func newLocalTarget(gw *Gateway) deploy.Target {
	return &localDeployTarget{gw: gw}
}

func (t *localDeployTarget) Deploy(ctx context.Context, s deploy.Spec) (deploy.Result, error) {
	// 1. Require Docker.
	dockerCheck := execCommand("docker", "version", "--format", "{{.Server.Version}}")
	out, err := dockerCheck.Output()
	if err != nil {
		return deploy.Result{}, fmt.Errorf("docker is not available on this host")
	}
	dockerVer := strings.TrimSpace(string(out))

	projectPath := s.ProjectPath
	slug := s.Slug
	if slug == "" {
		slug = sanitizeSlug(s.ProjectID)
	}
	imageName := fmt.Sprintf("qorven-deploy/%s", slug)

	// 2. Ensure a Dockerfile exists.
	dockerfilePath := filepath.Join(projectPath, "Dockerfile")
	if _, statErr := os.Stat(dockerfilePath); os.IsNotExist(statErr) {
		framework := s.Framework
		if framework == "" {
			framework, _ = detectFramework(projectPath)
		}
		if framework == "" {
			framework = "static"
		}
		content := generateDockerfile(framework, projectPath)
		if writeErr := os.WriteFile(dockerfilePath, []byte(content), 0644); writeErr != nil {
			return deploy.Result{}, fmt.Errorf("local deploy: write Dockerfile: %w", writeErr)
		}
	}

	// 3. Build the image.
	buildCmd := execCommand("docker", "build", "-t", imageName, "-f", "Dockerfile", ".")
	buildCmd.Dir = projectPath
	if buildOut, buildErr := buildCmd.CombinedOutput(); buildErr != nil {
		return deploy.Result{}, fmt.Errorf("local deploy: docker build failed: %w\n%s",
			buildErr, lastLines(string(buildOut), 5))
	}

	// 4. Stop any existing container with the same slug name.
	execCommand("docker", "rm", "-f", slug).Run() //nolint:errcheck

	// 5. Run the container on a deterministic host port.
	port := allocateDeployPort(slug)
	runCmd := execCommand("docker", "run", "-d",
		"--name", slug,
		"-p", fmt.Sprintf("127.0.0.1:%d:80", port),
		"--restart", "unless-stopped",
		imageName,
	)
	runOut, runErr := runCmd.CombinedOutput()
	if runErr != nil {
		return deploy.Result{}, fmt.Errorf("local deploy: docker run failed: %w — %s",
			runErr, strings.TrimSpace(string(runOut)))
	}

	// 6. Wait for the container to accept connections.
	if !waitForPort(port, 30*time.Second) {
		return deploy.Result{}, fmt.Errorf("local deploy: container did not become ready within 30 s")
	}

	url := fmt.Sprintf("http://127.0.0.1:%d", port)
	return deploy.Result{
		URL:    url,
		Target: "local",
		Detail: fmt.Sprintf("docker %s; image %s; port %d", dockerVer, imageName, port),
	}, nil
}
