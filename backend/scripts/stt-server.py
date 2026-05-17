#!/usr/bin/env python3
"""STT server for Qorven — accepts multipart form (like faster-whisper) or raw audio."""
import io, json, time, cgi, tempfile, os, whisper
from http.server import HTTPServer, BaseHTTPRequestHandler

MODEL = whisper.load_model("small")
print("Whisper small loaded, listening on :8881", flush=True)

class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        if self.path == "/transcribe":
            content_type = self.headers.get("Content-Type", "")
            length = int(self.headers.get("Content-Length", 0))
            
            if "multipart" in content_type:
                # Multipart form — extract "file" field
                form = cgi.FieldStorage(fp=self.rfile, headers=self.headers, environ={"REQUEST_METHOD": "POST", "CONTENT_TYPE": content_type})
                file_item = form["file"]
                audio_bytes = file_item.file.read()
                filename = file_item.filename or "audio.ogg"
            else:
                # Raw body
                audio_bytes = self.rfile.read(length)
                filename = "audio.wav"
            
            # Save to temp file
            ext = os.path.splitext(filename)[1] or ".ogg"
            tmp = tempfile.NamedTemporaryFile(suffix=ext, delete=False)
            tmp.write(audio_bytes)
            tmp.close()
            
            try:
                start = time.time()
                result = MODEL.transcribe(tmp.name, fp16=False, language="en")
                elapsed = time.time() - start
                text = result["text"].strip()
                print(f"Transcribed ({elapsed:.1f}s, {len(audio_bytes)}b, {ext}): {text[:80]}", flush=True)
                
                resp = json.dumps({"text": text, "language": result.get("language", "en")}).encode()
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.end_headers()
                self.wfile.write(resp)
            except Exception as e:
                print(f"STT error: {e}", flush=True)
                self.send_response(500)
                self.send_header("Content-Type", "application/json")
                self.end_headers()
                self.wfile.write(json.dumps({"error": str(e)}).encode())
            finally:
                os.unlink(tmp.name)
        else:
            self.send_response(404)
            self.end_headers()
    
    def do_GET(self):
        if self.path == "/health":
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(b'{"status":"ok","model":"whisper-small"}')
        else:
            self.send_response(404)
            self.end_headers()
    
    def log_message(self, format, *args):
        print(f"[STT] {args[0]}", flush=True)

HTTPServer(("0.0.0.0", 8881), Handler).serve_forever()
