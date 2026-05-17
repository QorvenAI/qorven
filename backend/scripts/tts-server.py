#!/usr/bin/env python3
"""TTS server for Qorven — piper on CPU, Kokoro-compatible API."""
import json, subprocess, tempfile, os, time
from http.server import HTTPServer, BaseHTTPRequestHandler

PIPER_MODEL = os.path.expanduser("~/.local/share/piper/en_US-lessac-medium.onnx")
PIPER_BIN = "piper"  # or full path

print(f"TTS server starting on :8880 (piper model: {PIPER_MODEL})", flush=True)

class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        if self.path == "/synthesize":
            length = int(self.headers.get("Content-Length", 0))
            body = json.loads(self.rfile.read(length)) if length else {}
            text = body.get("text", "")
            if not text:
                self.send_response(400)
                self.end_headers()
                self.wfile.write(b'{"error":"text required"}')
                return

            try:
                start = time.time()
                tmp = tempfile.NamedTemporaryFile(suffix=".wav", delete=False)
                tmp.close()

                proc = subprocess.run(
                    ["piper", "--model", PIPER_MODEL, "--output_file", tmp.name],
                    input=text.encode(), capture_output=True, timeout=30
                )

                if proc.returncode != 0:
                    self.send_response(500)
                    self.end_headers()
                    self.wfile.write(json.dumps({"error": proc.stderr.decode()[:200]}).encode())
                    os.unlink(tmp.name)
                    return

                with open(tmp.name, "rb") as f:
                    audio = f.read()
                os.unlink(tmp.name)

                elapsed = time.time() - start
                print(f"Synthesized ({elapsed:.1f}s, {len(audio)}b): {text[:60]}", flush=True)

                self.send_response(200)
                self.send_header("Content-Type", "audio/wav")
                self.send_header("Content-Length", str(len(audio)))
                self.end_headers()
                self.wfile.write(audio)

            except Exception as e:
                print(f"TTS error: {e}", flush=True)
                self.send_response(500)
                self.end_headers()
                self.wfile.write(json.dumps({"error": str(e)}).encode())
        else:
            self.send_response(404)
            self.end_headers()

    def do_GET(self):
        if self.path == "/health":
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(b'{"status":"ok","engine":"piper"}')
        else:
            self.send_response(404)
            self.end_headers()

    def log_message(self, format, *args):
        print(f"[TTS] {args[0]}", flush=True)

HTTPServer(("0.0.0.0", 8880), Handler).serve_forever()
