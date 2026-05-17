#!/usr/bin/env python3
"""
Qorven Voice Server — Kokoro TTS + Faster-Whisper STT
Deploy on a voice-dedicated host behind your gateway.

Runs on CPU. No GPU needed.
- Kokoro 82M: ~500MB RAM, Apache 2.0
- Faster-Whisper base: ~400MB RAM, MIT
- Total: ~1GB RAM

Install:
  pip install kokoro faster-whisper fastapi uvicorn python-multipart soundfile

Run:
  python voice_server.py
  # or
  uvicorn voice_server:app --host 0.0.0.0 --port 8880

Endpoints:
  POST /synthesize  — text → audio (Kokoro TTS)
  POST /transcribe  — audio → text (Faster-Whisper STT)
  GET  /health      — health check
  GET  /voices      — list available voices
"""

import io
import os
import tempfile
import logging
from fastapi import FastAPI, UploadFile, File, Form
from fastapi.responses import StreamingResponse, JSONResponse

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger("voice")

app = FastAPI(title="Qorven Voice Server", version="1.0.0")

# --- Lazy-load models (download on first use) ---

_kokoro_pipeline = None
_whisper_model = None

def get_kokoro():
    global _kokoro_pipeline
    if _kokoro_pipeline is None:
        logger.info("Loading Kokoro TTS (82M)...")
        from kokoro import KPipeline
        _kokoro_pipeline = KPipeline(lang_code='a')  # American English
        logger.info("Kokoro TTS loaded")
    return _kokoro_pipeline

def get_whisper(model_size: str = "base"):
    global _whisper_model
    if _whisper_model is None:
        logger.info(f"Loading Faster-Whisper ({model_size})...")
        from faster_whisper import WhisperModel
        _whisper_model = WhisperModel(model_size, device="cpu", compute_type="int8")
        logger.info(f"Faster-Whisper ({model_size}) loaded")
    return _whisper_model

# --- TTS Endpoint ---

@app.post("/synthesize")
async def synthesize(
    text: str = Form(None),
    voice: str = Form("af_heart"),
    speed: float = Form(1.0),
):
    """Convert text to speech using Kokoro 82M."""
    # Also accept JSON body
    if text is None:
        from fastapi import Request
        # Will be handled by JSON body below
        pass

    import soundfile as sf
    import numpy as np

    pipeline = get_kokoro()
    
    # Generate audio
    audio_chunks = []
    for _, _, audio in pipeline(text, voice=voice, speed=speed):
        audio_chunks.append(audio)
    
    if not audio_chunks:
        return JSONResponse({"error": "no audio generated"}, status_code=500)
    
    # Concatenate chunks
    full_audio = np.concatenate(audio_chunks)
    
    # Write to WAV buffer
    buf = io.BytesIO()
    sf.write(buf, full_audio, 24000, format='WAV')
    buf.seek(0)
    
    return StreamingResponse(buf, media_type="audio/wav",
        headers={"Content-Disposition": "inline; filename=speech.wav"})

@app.post("/synthesize_json")
async def synthesize_json(body: dict):
    """JSON body version of synthesize."""
    text = body.get("text", "")
    voice = body.get("voice", "af_heart")
    speed = body.get("speed", 1.0)
    
    if not text:
        return JSONResponse({"error": "text required"}, status_code=400)
    
    import soundfile as sf
    import numpy as np

    pipeline = get_kokoro()
    audio_chunks = []
    for _, _, audio in pipeline(text, voice=voice, speed=speed):
        audio_chunks.append(audio)
    
    if not audio_chunks:
        return JSONResponse({"error": "no audio generated"}, status_code=500)
    
    full_audio = np.concatenate(audio_chunks)
    buf = io.BytesIO()
    sf.write(buf, full_audio, 24000, format='WAV')
    buf.seek(0)
    
    return StreamingResponse(buf, media_type="audio/wav")

# --- STT Endpoint ---

@app.post("/transcribe")
async def transcribe(
    file: UploadFile = File(...),
    model: str = Form("base"),
):
    """Transcribe audio to text using Faster-Whisper."""
    whisper = get_whisper(model)
    
    # Save uploaded file to temp
    with tempfile.NamedTemporaryFile(suffix=f".{file.filename.split('.')[-1] if file.filename else 'webm'}", delete=False) as tmp:
        content = await file.read()
        tmp.write(content)
        tmp_path = tmp.name
    
    try:
        segments, info = whisper.transcribe(tmp_path, beam_size=5)
        text = " ".join(segment.text for segment in segments).strip()
        return {"text": text, "language": info.language, "duration": info.duration}
    finally:
        os.unlink(tmp_path)

# --- Info Endpoints ---

@app.get("/health")
async def health():
    return {"status": "ok", "tts": "kokoro-82m", "stt": "faster-whisper"}

@app.get("/voices")
async def voices():
    """List available Kokoro voices."""
    return {"voices": [
        {"id": "af_heart", "name": "Heart (Female)", "lang": "en-US"},
        {"id": "af_bella", "name": "Bella (Female)", "lang": "en-US"},
        {"id": "af_nicole", "name": "Nicole (Female)", "lang": "en-US"},
        {"id": "af_sarah", "name": "Sarah (Female)", "lang": "en-US"},
        {"id": "af_sky", "name": "Sky (Female)", "lang": "en-US"},
        {"id": "am_adam", "name": "Adam (Male)", "lang": "en-US"},
        {"id": "am_michael", "name": "Michael (Male)", "lang": "en-US"},
        {"id": "bf_emma", "name": "Emma (Female)", "lang": "en-GB"},
        {"id": "bm_george", "name": "George (Male)", "lang": "en-GB"},
    ]}

if __name__ == "__main__":
    import uvicorn
    port = int(os.environ.get("PORT", "8880"))
    logger.info(f"Starting Qorven Voice Server on port {port}")
    uvicorn.run(app, host="0.0.0.0", port=port)
