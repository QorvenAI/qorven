//go:build ignore

// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

// testdata/echo_plugin.go — sample Wasm plugin used by the host test
// suite. Compile with:
//
//   GOOS=wasip1 GOARCH=wasm go build -o testdata/echo_plugin.wasm \
//     testdata/echo_plugin.go
//
// The Makefile target `wasm-testdata` wraps that command. The
// compiled .wasm is checked in so CI (which doesn't run `go build`
// for wasip1 in the verify target) picks it up directly.
//
// Contract (mirrors the host's expectations — see ../host.go):
//   • Read a JSON payload from STDIN.
//   • Write a JSON reply to STDOUT.
//   • Exit 0 on success, exit 1 on structured error (and write a
//     message to STDERR first).

package main

import (
	"encoding/json"
	"io"
	"os"
)

// echoRequest is the shape the host will send. For the test we just
// echo the `message` field back plus a few static fields so the host
// can assert the round-trip actually executed guest code.
type echoRequest struct {
	Message string `json:"message"`
	// Triggers the exit(1) branch for testing the error path.
	FailWith string `json:"fail_with,omitempty"`
	// Triggers an infinite loop for testing the timeout path.
	Spin bool `json:"spin,omitempty"`
	// Triggers an oversized stdout for testing the truncation path.
	BigReply bool `json:"big_reply,omitempty"`
}

type echoReply struct {
	Echoed    string `json:"echoed"`
	FromWasm  bool   `json:"from_wasm"`
	InputLen  int    `json:"input_len"`
	Big       string `json:"big,omitempty"`
}

func main() {
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Stderr.WriteString("wasm plugin: read stdin: " + err.Error())
		os.Exit(1)
	}
	var req echoRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		os.Stderr.WriteString("wasm plugin: bad json: " + err.Error())
		os.Exit(1)
	}

	if req.FailWith != "" {
		os.Stderr.WriteString(req.FailWith)
		os.Exit(1)
	}

	if req.Spin {
		// Busy loop — the host timeout must cut us off.
		for {
			_ = req.Message
		}
	}

	reply := echoReply{
		Echoed:   req.Message,
		FromWasm: true,
		InputLen: len(raw),
	}
	if req.BigReply {
		// Bypass json.Marshal for the big reply path — under the
		// Wasm interpreter, marshaling 64 KiB of string literals
		// is measurable. We emit a small JSON header then raw
		// padding bytes direct to stdout; the HOST's limitedBuffer
		// truncates partway through and the guest exits cleanly.
		os.Stdout.WriteString(`{"from_wasm":true,"big":"`)
		pad := make([]byte, 4096)
		for i := range pad {
			pad[i] = 'x'
		}
		// 16 × 4 KiB = 64 KiB — exceeds the 16 KiB test cap by 4×
		// while keeping total IO under a budget the interpreter
		// can cover in 2s.
		for i := 0; i < 16; i++ {
			os.Stdout.Write(pad)
		}
		os.Stdout.WriteString(`"}`)
		return
	}

	out, err := json.Marshal(reply)
	if err != nil {
		os.Stderr.WriteString("wasm plugin: marshal reply: " + err.Error())
		os.Exit(1)
	}
	os.Stdout.Write(out)
}
