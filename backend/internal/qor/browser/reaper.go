// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package browser

import (
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// reaper.go — Cleanup zombie browser processes that outlive their sessions.

// StartReaper runs a background goroutine that kills stale Chrome processes.
func StartReaper(interval time.Duration, maxAge time.Duration) {
	if interval <= 0 { interval = 5 * time.Minute }
	if maxAge <= 0 { maxAge = 30 * time.Minute }

	go func() {
		for {
			time.Sleep(interval)
			killed := killStaleChrome(maxAge)
			if killed > 0 {
				slog.Info("browser.reaper: killed stale processes", "count", killed)
			}
		}
	}()
}

func killStaleChrome(maxAge time.Duration) int {
	// Find chrome/chromium processes
	out, err := exec.Command("ps", "-eo", "pid,etimes,comm").Output()
	if err != nil { return 0 }

	killed := 0
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 { continue }
		comm := fields[2]
		if !strings.Contains(comm, "chrome") && !strings.Contains(comm, "chromium") { continue }

		elapsed, err := strconv.Atoi(fields[1])
		if err != nil { continue }

		if time.Duration(elapsed)*time.Second > maxAge {
			pid, _ := strconv.Atoi(fields[0])
			if pid > 0 {
				exec.Command("kill", "-9", strconv.Itoa(pid)).Run()
				killed++
			}
		}
	}
	return killed
}
