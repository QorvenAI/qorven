// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

// Deploy-target: hosted
//
// Reachability decision (tunnel vs. sandbox proxy):
//   - enableTunnel / the tunnel manager expose port 8487 (the public mux built by
//     buildPublicMux).  The sandbox proxy route (/sandbox/{prefix}/) is mounted on
//     the MAIN mux (port 4200), NOT on the public mux.  Therefore the tunnel cannot
//     reach AppRunner containers.
//   - PRIMARY URL = app.ProxyURL  (gateway base + /sandbox/<prefix>/), always
//     reachable for any authenticated client that can reach the gateway.
//   - Tunnel: enabled best-effort (cloudflare quick tunnel) so external visitors
//     can reach Qorven's own public apps surface; it does NOT expose the sandbox
//     proxy and is therefore noted in Detail only.
//
// AppRunner build semantics:
//   AppRunner.Start resolves its ImageOrRepo by calling either buildFromGit (for
//   https:// / git@ URLs) or docker pull (for everything else).  docker pull fails
//   for locally built images that are not in a registry.  Therefore we cannot pass
//   a locally built image tag directly to AppRunner.Start.
//   Instead we:
//     1. docker build -t qorven-deploy/<slug>  (reusing deploy_handler helpers)
//     2. docker run -d -p 0:80  (let Docker assign a random host port)
//     3. docker inspect to get the actual host port
//     4. AppRunner.RegisterRunningContainer — registers the container + host port
//        in DB and the in-memory route table, returns a *RunningApp with ProxyURL.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/qorvenai/qorven/internal/gateway/deploy"
	"github.com/qorvenai/qorven/internal/sandbox"
)

// hostedDeployTarget builds a Docker image, runs it, and registers it with the
// AppRunner so it is reachable through the gateway's /sandbox/ reverse proxy.
type hostedDeployTarget struct {
	gw *Gateway
}

// newHostedTarget constructs a hostedDeployTarget.
func newHostedTarget(gw *Gateway) deploy.Target {
	return &hostedDeployTarget{gw: gw}
}

func (t *hostedDeployTarget) Deploy(ctx context.Context, s deploy.Spec) (deploy.Result, error) {
	if t.gw.appRunner == nil {
		return deploy.Result{}, fmt.Errorf("hosted deploy: app runner not initialised (database required)")
	}

	// 1. Require Docker.
	dockerCheck := execCommand("docker", "version", "--format", "{{.Server.Version}}")
	if out, err := dockerCheck.Output(); err != nil {
		return deploy.Result{}, fmt.Errorf("docker is not available on this host")
	} else {
		_ = out // version checked; proceed
	}

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
			return deploy.Result{}, fmt.Errorf("hosted deploy: write Dockerfile: %w", writeErr)
		}
	}

	// 3. Build the image.
	buildCmd := execCommand("docker", "build", "-t", imageName, "-f", "Dockerfile", ".")
	buildCmd.Dir = projectPath
	if buildOut, buildErr := buildCmd.CombinedOutput(); buildErr != nil {
		return deploy.Result{}, fmt.Errorf("hosted deploy: docker build failed: %w\n%s",
			buildErr, lastLines(string(buildOut), 5))
	}

	// 4. Stop any previously registered container for this slug.
	execCommand("docker", "rm", "-f", "qorven-hosted-"+slug).Run() //nolint:errcheck

	// 5. Run the container — let Docker assign a random host port (-p 0:80).
	containerName := "qorven-hosted-" + slug
	runCmd := execCommand("docker", "run", "-d",
		"--name", containerName,
		"--label", "qorven.app=true",
		"--label", "qorven.hosted=true",
		"--network", "bridge",
		"--security-opt", "no-new-privileges",
		"--memory", "1g",
		"--cpus", "1.0",
		"-p", "0:80",
		imageName,
	)
	runOut, runErr := runCmd.CombinedOutput()
	if runErr != nil {
		return deploy.Result{}, fmt.Errorf("hosted deploy: docker run failed: %w — %s",
			runErr, strings.TrimSpace(string(runOut)))
	}
	containerID := strings.TrimSpace(string(runOut))
	if len(containerID) > 12 {
		containerID = containerID[:12]
	}

	// 6. Discover the host port Docker assigned.
	hostPort, err := inspectHostPort(ctx, containerID, 80)
	if err != nil {
		execCommand("docker", "rm", "-f", containerID).Run() //nolint:errcheck
		return deploy.Result{}, fmt.Errorf("hosted deploy: get host port: %w", err)
	}

	// 7. Wait for the container to be ready.
	if !waitForPort(hostPort, 30*time.Second) {
		execCommand("docker", "rm", "-f", containerID).Run() //nolint:errcheck
		return deploy.Result{}, fmt.Errorf("hosted deploy: container did not become ready within 30 s")
	}

	// 8. Register the running container with AppRunner to get a proxy route.
	app, err := t.gw.appRunner.RegisterRunningContainer(ctx, sandbox.RunAppParams{
		TenantID:    defaultTenant,
		ImageOrRepo: imageName,
		Label:       slug,
		Port:        80,
		TTLMinutes:  0, // persistent (RegisterRunningContainer maps 0 → 480 min max)
		BaseURL:     t.gw.cfg.Server.BaseURL,
	}, containerID, hostPort)
	if err != nil {
		execCommand("docker", "rm", "-f", containerID).Run() //nolint:errcheck
		return deploy.Result{}, fmt.Errorf("hosted deploy: register container: %w", err)
	}

	// 9. Best-effort public tunnel (cloudflare quick tunnel).
	//    The tunnel exposes port 8487 (the public mux) — it does NOT reach the
	//    sandbox proxy on the main mux.  We enable it for Qorven's own public apps
	//    surface but document that app.ProxyURL is the primary reachable URL.
	tunnelDetail := "tunnel: not started"
	if t.gw.tunnelMgr != nil {
		if tunnelErr := t.gw.enableTunnel("cloudflare"); tunnelErr == nil {
			tunnelDetail = "tunnel: starting (exposes public mux at :8487, not the sandbox proxy)"
		} else {
			tunnelDetail = fmt.Sprintf("tunnel: unavailable (%v)", tunnelErr)
		}
	}

	return deploy.Result{
		URL:    app.ProxyURL,
		Target: "hosted",
		Detail: fmt.Sprintf("proxy %s; %s", app.ProxyURL, tunnelDetail),
	}, nil
}

// inspectHostPort uses docker inspect to find the host port bound to internalPort
// inside the given container.
func inspectHostPort(ctx context.Context, containerID string, internalPort int) (int, error) {
	portSpec := fmt.Sprintf("%d/tcp", internalPort)
	format := fmt.Sprintf(`{{(index (index .NetworkSettings.Ports "%s") 0).HostPort}}`, portSpec)
	out, err := execCommand("docker", "inspect", "--format", format, containerID).Output()
	if err != nil {
		return 0, fmt.Errorf("docker inspect: %w", err)
	}
	port, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0, fmt.Errorf("parse host port %q: %w", strings.TrimSpace(string(out)), err)
	}
	return port, nil
}
