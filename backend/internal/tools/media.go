// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/qorvenai/qorven/internal/mediagen"
	"github.com/qorvenai/qorven/internal/providers"
	"github.com/qorvenai/qorven/internal/voice"
)

// Media tools proxy to LLM providers for vision, generation, TTS.
// Each tool finds the right provider and delegates.

// --- read_image (vision) ---

type ReadImageTool struct{ reg *providers.Registry }

func NewReadImageTool(reg *providers.Registry) *ReadImageTool { return &ReadImageTool{reg: reg} }
func (t *ReadImageTool) Name() string                        { return "read_image" }
func (t *ReadImageTool) Description() string                 { return "Analyze images using a vision-capable LLM." }
func (t *ReadImageTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"image_path": map[string]any{"type": "string", "description": "Path to image file"},
		"question":   map[string]any{"type": "string", "description": "What to analyze (default: describe the image)"},
	}, "required": []string{"image_path"}}
}

func (t *ReadImageTool) Execute(ctx context.Context, args map[string]any) *Result {
	if t.reg == nil {
		return ErrorResult("read_image: no provider registry available")
	}
	p := t.reg.Default()
	if p == nil {
		return ErrorResult("read_image: no LLM provider configured — add one in Settings → Provider Keys")
	}
	imagePath, _ := args["image_path"].(string)
	if imagePath == "" {
		return ErrorResult("read_image: image_path is required")
	}
	question, _ := args["question"].(string)
	if question == "" {
		question = "Describe this image in detail."
	}

	data, err := os.ReadFile(imagePath)
	if err != nil {
		return ErrorResult(fmt.Sprintf("read_image: cannot read file %q: %v", imagePath, err))
	}

	// Detect MIME type from extension, fall back to sniffing.
	mimeType := mimeFromExt(filepath.Ext(imagePath))
	if mimeType == "" {
		mimeType = http.DetectContentType(data)
	}
	if !strings.HasPrefix(mimeType, "image/") {
		return ErrorResult(fmt.Sprintf("read_image: %q does not appear to be an image (detected %s)", imagePath, mimeType))
	}

	resp, err := p.Chat(ctx, providers.ChatRequest{
		Messages: []providers.Message{
			{
				Role:    "user",
				Content: question,
				Images:  []providers.ImageContent{{MimeType: mimeType, Data: base64.StdEncoding.EncodeToString(data)}},
			},
		},
	})
	if err != nil {
		return ErrorResult(fmt.Sprintf("read_image: vision call failed: %v", err))
	}
	return TextResult(resp.Content)
}

func mimeFromExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	default:
		return ""
	}
}

// --- create_image ---

type CreateImageTool struct {
	reg *providers.Registry
	mgr *mediagen.Manager
}

func NewCreateImageTool(reg *providers.Registry, mgr *mediagen.Manager) *CreateImageTool {
	return &CreateImageTool{reg: reg, mgr: mgr}
}
func (t *CreateImageTool) Name() string        { return "create_image" }
func (t *CreateImageTool) Description() string { return "Generate images from text descriptions using an image generation provider (DALL-E, Stability AI, FLUX, etc.)." }
func (t *CreateImageTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"prompt":  map[string]any{"type": "string", "description": "Detailed image description"},
		"size":    map[string]any{"type": "string", "description": "Image size: 1024x1024 (square, default), 1792x1024 (landscape), 1024x1792 (portrait)", "enum": []string{"1024x1024", "1792x1024", "1024x1792"}},
		"quality": map[string]any{"type": "string", "description": "Quality: standard (default) or hd", "enum": []string{"standard", "hd"}},
		"style":   map[string]any{"type": "string", "description": "Style: vivid (default) or natural", "enum": []string{"vivid", "natural"}},
	}, "required": []string{"prompt"}}
}

func (t *CreateImageTool) Execute(ctx context.Context, args map[string]any) *Result {
	if t.mgr == nil || !t.mgr.HasImage() {
		return ErrorResult("create_image: no image generation provider configured — add one in Settings → Models Hub → Media tab")
	}
	prompt, _ := args["prompt"].(string)
	if prompt == "" {
		return ErrorResult("create_image: prompt is required")
	}
	size, _ := args["size"].(string)
	quality, _ := args["quality"].(string)
	style, _ := args["style"].(string)

	result, err := t.mgr.GenerateImage(ctx, prompt, mediagen.ImageOptions{
		Size: size, Quality: quality, Style: style,
	})
	if err != nil {
		return ErrorResult(fmt.Sprintf("create_image: %v", err))
	}

	// Build src: prefer URL, fall back to inline base64
	src := result.URL
	if src == "" && result.B64JSON != "" {
		mime := result.MimeType
		if mime == "" {
			mime = "image/png"
		}
		src = "data:" + mime + ";base64," + result.B64JSON
	}
	if src == "" {
		return ErrorResult("create_image: provider returned no image data")
	}

	return &Result{
		ForLLM:  fmt.Sprintf("Image generated for prompt: %q. Rendered inline in chat.", prompt),
		ForUser: "🎨 Image generated",
		Widget: &Widget{
			Type: "image",
			Data: map[string]any{
				"url":    src,
				"prompt": prompt,
			},
		},
	}
}

// --- read_document ---

type ReadDocumentTool struct{}

func NewReadDocumentTool() *ReadDocumentTool { return &ReadDocumentTool{} }
func (t *ReadDocumentTool) Name() string     { return "read_document" }
func (t *ReadDocumentTool) Description() string {
	return "Analyze documents (PDF, DOCX, etc.). If this fails, use a relevant skill instead."
}
func (t *ReadDocumentTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"path":     map[string]any{"type": "string", "description": "Document file path"},
		"question": map[string]any{"type": "string", "description": "What to extract or analyze"},
	}, "required": []string{"path"}}
}

func (t *ReadDocumentTool) Execute(ctx context.Context, args map[string]any) *Result {
	return ErrorResult("read_document: not yet implemented — use skill_search to find a pdf or docx skill, or use web_fetch on a public URL")
}

// --- read_audio, read_video, create_audio, create_video, tts (stubs wired to providers) ---

func newMediaStub(name, desc string) Tool {
	return &mediaStub{name: name, desc: desc}
}

type mediaStub struct{ name, desc string }

func (t *mediaStub) Name() string        { return t.name }
func (t *mediaStub) Description() string { return t.desc }
func (t *mediaStub) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"input": map[string]any{"type": "string", "description": "Input text or file path"},
	}, "required": []string{"input"}}
}
func (t *mediaStub) Execute(ctx context.Context, args map[string]any) *Result {
	return ErrorResult(fmt.Sprintf("%s: not yet implemented — configure a suitable provider in Settings → Provider Keys", t.name))
}

func NewReadAudioTool(mgr *voice.Manager) Tool { return &ReadAudioTool{mgr: mgr} }

type ReadAudioTool struct{ mgr *voice.Manager }

func (t *ReadAudioTool) Name() string        { return "read_audio" }
func (t *ReadAudioTool) Description() string { return "Transcribe an audio file to text using STT." }
func (t *ReadAudioTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"audio_path": map[string]any{"type": "string", "description": "Path to audio file (mp3, wav, m4a, ogg, flac)"},
	}, "required": []string{"audio_path"}}
}
func (t *ReadAudioTool) Execute(ctx context.Context, args map[string]any) *Result {
	if t.mgr == nil || !t.mgr.HasSTT() {
		return ErrorResult("read_audio: no STT provider configured — add Whisper or Deepgram in Settings → Provider Keys")
	}
	audioPath, _ := args["audio_path"].(string)
	if audioPath == "" {
		return ErrorResult("read_audio: audio_path is required")
	}
	data, err := os.ReadFile(audioPath)
	if err != nil {
		return ErrorResult(fmt.Sprintf("read_audio: cannot read file %q: %v", audioPath, err))
	}
	format := strings.TrimPrefix(strings.ToLower(filepath.Ext(audioPath)), ".")
	transcript, err := t.mgr.Transcribe(ctx, data, format)
	if err != nil {
		return ErrorResult(fmt.Sprintf("read_audio: transcription failed: %v", err))
	}
	return TextResult(transcript)
}
func NewReadVideoTool(reg *providers.Registry) Tool { return &ReadVideoTool{reg: reg} }

type ReadVideoTool struct{ reg *providers.Registry }

func (t *ReadVideoTool) Name() string        { return "read_video" }
func (t *ReadVideoTool) Description() string { return "Analyze a video file by extracting a key frame and asking a vision-capable LLM about it. Requires ffmpeg on the system path." }
func (t *ReadVideoTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"video_path": map[string]any{"type": "string", "description": "Path to the video file (mp4, mkv, mov, avi, webm)"},
		"question":   map[string]any{"type": "string", "description": "What to analyze or describe (default: describe the video scene)"},
		"timestamp":  map[string]any{"type": "string", "description": "Timestamp to extract frame from (HH:MM:SS or seconds, default: 00:00:01)"},
	}, "required": []string{"video_path"}}
}

func (t *ReadVideoTool) Execute(ctx context.Context, args map[string]any) *Result {
	if t.reg == nil {
		return ErrorResult("read_video: no provider registry available")
	}
	p := t.reg.Default()
	if p == nil {
		return ErrorResult("read_video: no LLM provider configured — add one in Settings → Provider Keys")
	}
	videoPath, _ := args["video_path"].(string)
	if videoPath == "" {
		return ErrorResult("read_video: video_path is required")
	}
	if _, err := os.Stat(videoPath); err != nil {
		return ErrorResult(fmt.Sprintf("read_video: cannot access %q: %v", videoPath, err))
	}
	question, _ := args["question"].(string)
	if question == "" {
		question = "Describe what is happening in this video frame in detail."
	}
	timestamp, _ := args["timestamp"].(string)
	if timestamp == "" {
		timestamp = "00:00:01"
	}

	// Extract a single frame with ffmpeg.
	frameFile, err := os.CreateTemp("", "qorven-video-frame-*.jpg")
	if err != nil {
		return ErrorResult(fmt.Sprintf("read_video: failed to create temp file: %v", err))
	}
	framePath := frameFile.Name()
	frameFile.Close()
	defer os.Remove(framePath)

	// #nosec G204 — args are validated above (file exists, timestamp is caller-supplied but not shell-expanded)
	ffmpegOut, execErr := execContext(ctx, "ffmpeg",
		"-y", "-ss", timestamp, "-i", videoPath, "-vframes", "1", "-q:v", "2", framePath,
	)
	if execErr != nil {
		return ErrorResult(fmt.Sprintf("read_video: ffmpeg frame extraction failed: %v\n%s", execErr, string(ffmpegOut)))
	}

	data, err := os.ReadFile(framePath)
	if err != nil || len(data) == 0 {
		return ErrorResult("read_video: frame extraction produced no output — check that ffmpeg is installed and the file is a valid video")
	}

	resp, err := p.Chat(ctx, providers.ChatRequest{
		Messages: []providers.Message{
			{
				Role:    "user",
				Content: question,
				Images:  []providers.ImageContent{{MimeType: "image/jpeg", Data: base64.StdEncoding.EncodeToString(data)}},
			},
		},
	})
	if err != nil {
		return ErrorResult(fmt.Sprintf("read_video: vision call failed: %v", err))
	}
	return TextResult(fmt.Sprintf("[Frame at %s] %s", timestamp, resp.Content))
}

func execContext(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...) // #nosec G204
	return cmd.CombinedOutput()
}


// NewCreateVideoTool wires create_video to a mediagen.Manager video provider.
func NewCreateVideoTool(mgr *mediagen.Manager) Tool { return &CreateVideoTool{mgr: mgr} }

type CreateVideoTool struct{ mgr *mediagen.Manager }

func (t *CreateVideoTool) Name() string        { return "create_video" }
func (t *CreateVideoTool) Description() string { return "Generate a video from a text description using a video generation provider (Runway ML, Kling AI)." }
func (t *CreateVideoTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"prompt":       map[string]any{"type": "string", "description": "Detailed description of the video scene"},
		"duration":     map[string]any{"type": "integer", "description": "Duration in seconds (5 or 10, default 5)"},
		"aspect_ratio": map[string]any{"type": "string", "description": "Aspect ratio: 16:9 (default), 9:16 (vertical), 1:1 (square)", "enum": []string{"16:9", "9:16", "1:1"}},
		"image_url":    map[string]any{"type": "string", "description": "Optional URL of a reference/first-frame image"},
	}, "required": []string{"prompt"}}
}

func (t *CreateVideoTool) Execute(ctx context.Context, args map[string]any) *Result {
	if t.mgr == nil || !t.mgr.HasVideo() {
		return ErrorResult("create_video: no video generation provider configured — add Runway ML or Kling AI in Settings → Models Hub → Media tab")
	}
	prompt, _ := args["prompt"].(string)
	if prompt == "" {
		return ErrorResult("create_video: prompt is required")
	}
	dur := 5
	if d, ok := args["duration"].(float64); ok && d > 0 {
		dur = int(d)
	}
	ratio, _ := args["aspect_ratio"].(string)
	imageURL, _ := args["image_url"].(string)

	result, err := t.mgr.GenerateVideo(ctx, prompt, mediagen.VideoOptions{
		Duration: dur, AspectRatio: ratio, ImageURL: imageURL,
	})
	if err != nil {
		return ErrorResult(fmt.Sprintf("create_video: %v", err))
	}

	if result.URL != "" {
		return &Result{
			ForLLM:  fmt.Sprintf("Video generated for prompt: %q. URL: %s", prompt, result.URL),
			ForUser: "🎬 Video generated",
			Widget: &Widget{
				Type: "video",
				Data: map[string]any{
					"url":    result.URL,
					"prompt": prompt,
				},
			},
		}
	}
	// Async — return task ID for user to retrieve
	return TextResult(fmt.Sprintf("Video generation started (task ID: %s). The provider is processing your request — it may take 1–3 minutes. Poll URL: %s", result.TaskID, result.PollURL))
}

// --- tts (text-to-speech) ---

type TTSTool struct{ mgr *voice.Manager }

func NewTTSTool(mgr *voice.Manager) *TTSTool { return &TTSTool{mgr: mgr} }
func (t *TTSTool) Name() string              { return "tts" }
func (t *TTSTool) Description() string {
	return "Convert text to speech. Returns an audio widget the user can play in the chat."
}
func (t *TTSTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"text":   map[string]any{"type": "string", "description": "Text to speak"},
		"voice":  map[string]any{"type": "string", "description": "Voice name (provider-specific, optional)"},
		"format": map[string]any{"type": "string", "description": "Audio format: mp3, wav, opus (default: mp3)"},
	}, "required": []string{"text"}}
}

func (t *TTSTool) Execute(ctx context.Context, args map[string]any) *Result {
	if t.mgr == nil || !t.mgr.HasTTS() {
		return ErrorResult("tts: no TTS provider configured — add EdgeTTS or OpenAI TTS in Settings → Provider Keys")
	}
	text, _ := args["text"].(string)
	if text == "" {
		return ErrorResult("tts: text is required")
	}
	voiceName, _ := args["voice"].(string)
	format, _ := args["format"].(string)
	if format == "" {
		format = "mp3"
	}

	result, err := t.mgr.Synthesize(ctx, text, voice.TTSOptions{Voice: voiceName, Format: format})
	if err != nil {
		return ErrorResult(fmt.Sprintf("tts: synthesis failed: %v", err))
	}

	audioB64 := base64.StdEncoding.EncodeToString(result.Audio)
	mimeType := result.MimeType
	if mimeType == "" {
		mimeType = "audio/" + result.Extension
	}

	return &Result{
		ForLLM:  fmt.Sprintf("Audio generated (%d bytes, %s). Rendered as audio player in chat.", len(result.Audio), result.Extension),
		ForUser: fmt.Sprintf("🔊 Audio (%d bytes)", len(result.Audio)),
		Widget: &Widget{
			Type: "audio",
			Data: map[string]any{
				"src":       "data:" + mimeType + ";base64," + audioB64,
				"mime_type": mimeType,
				"text":      text,
			},
		},
	}
}

// --- create_audio (TTS → file) ---

type CreateAudioTool struct{ mgr *voice.Manager }

func NewCreateAudioTool(mgr *voice.Manager) *CreateAudioTool { return &CreateAudioTool{mgr: mgr} }
func (t *CreateAudioTool) Name() string                      { return "create_audio" }
func (t *CreateAudioTool) Description() string {
	return "Convert text to speech and save the result to a file. Returns the file path for downstream use (attach to email, save to drive, etc.)."
}
func (t *CreateAudioTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"text":     map[string]any{"type": "string", "description": "Text to synthesise"},
		"filename": map[string]any{"type": "string", "description": "Output filename without extension, e.g. 'welcome' (default: audio_<timestamp>)"},
		"voice":    map[string]any{"type": "string", "description": "Voice name (provider-specific, optional)"},
		"format":   map[string]any{"type": "string", "description": "Audio format: mp3, wav, opus (default: mp3)"},
	}, "required": []string{"text"}}
}

func (t *CreateAudioTool) Execute(ctx context.Context, args map[string]any) *Result {
	if t.mgr == nil || !t.mgr.HasTTS() {
		return ErrorResult("create_audio: no TTS provider configured — add EdgeTTS or OpenAI TTS in Settings → Provider Keys")
	}
	text, _ := args["text"].(string)
	if text == "" {
		return ErrorResult("create_audio: text is required")
	}
	voiceName, _ := args["voice"].(string)
	format, _ := args["format"].(string)
	if format == "" {
		format = "mp3"
	}
	filename, _ := args["filename"].(string)
	if filename == "" {
		filename = fmt.Sprintf("audio_%d", time.Now().UnixMilli())
	}

	result, err := t.mgr.Synthesize(ctx, text, voice.TTSOptions{Voice: voiceName, Format: format})
	if err != nil {
		return ErrorResult(fmt.Sprintf("create_audio: synthesis failed: %v", err))
	}

	ext := result.Extension
	if ext == "" {
		ext = format
	}
	outPath := filepath.Join(os.TempDir(), filename+"."+ext)
	if werr := os.WriteFile(outPath, result.Audio, 0600); werr != nil {
		return ErrorResult(fmt.Sprintf("create_audio: failed to write file: %v", werr))
	}

	return TextResult(fmt.Sprintf("Audio saved to %s (%d bytes, %s)", outPath, len(result.Audio), ext))
}

// --- message (send to channel) ---

type MessageTool struct{}

func NewMessageTool() *MessageTool    { return &MessageTool{} }
func (t *MessageTool) Name() string   { return "message" }
func (t *MessageTool) Description() string {
	return "Send a proactive message to a channel/chat. Do NOT use this to reply — just respond directly."
}
func (t *MessageTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"channel": map[string]any{"type": "string", "description": "Channel: telegram, discord, slack, web"},
		"chat_id": map[string]any{"type": "string", "description": "Chat/channel ID"},
		"text":    map[string]any{"type": "string", "description": "Message text"},
	}, "required": []string{"channel", "chat_id", "text"}}
}

func (t *MessageTool) Execute(ctx context.Context, args map[string]any) *Result {
	return ErrorResult("message: no channels connected — configure a channel in Settings → Channels first")
}

// --- spawn (subagent) ---

type SpawnTool struct{}

func NewSpawnTool() *SpawnTool      { return &SpawnTool{} }
func (t *SpawnTool) Name() string   { return "spawn" }
func (t *SpawnTool) Description() string {
	return "Spawn a subagent to handle a task in the background. The subagent inherits your tools and context."
}
func (t *SpawnTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"task":        map[string]any{"type": "string", "description": "Task description for the subagent"},
		"agent_key":   map[string]any{"type": "string", "description": "Agent to spawn (default: self-clone)"},
		"background":  map[string]any{"type": "boolean", "description": "Run in background (default: true)"},
	}, "required": []string{"task"}}
}

func (t *SpawnTool) Execute(ctx context.Context, args map[string]any) *Result {
	task, _ := args["task"].(string)
	if task == "" {
		return ErrorResult("task is required")
	}
	fork := ForkFuncFromCtx(ctx)
	if fork == nil {
		return ErrorResult("spawn: fork not available in this context")
	}
	result, err := fork(ctx, task)
	if err != nil {
		return ErrorResult(fmt.Sprintf("spawn failed: %v", err))
	}
	return TextResult(result)
}

// --- team_tasks ---

// TeamTasksBackend is the narrow interface this tool needs from tasks.Store.
type TeamTasksBackend interface {
	ListAll(ctx context.Context, tenantID, status string, limit int) ([]TeamTaskRow, error)
	CreateTask(ctx context.Context, tenantID, title string) (string, error)
	Transition(ctx context.Context, id, newStatus string) error
	CompleteTask(ctx context.Context, id, result string) error
}

// TeamTaskRow is the minimal shape this tool reads from the store.
type TeamTaskRow struct {
	ID         string
	Title      string
	Status     string
	AssignedTo *string
}

type TeamTasksTool struct {
	backend  TeamTasksBackend
	tenantID string
}

func NewTeamTasksTool(backend TeamTasksBackend, tenantID string) *TeamTasksTool {
	return &TeamTasksTool{backend: backend, tenantID: tenantID}
}
func (t *TeamTasksTool) Name() string { return "team_tasks" }
func (t *TeamTasksTool) Description() string {
	return "Team task board — list, create, update status, or complete tasks."
}
func (t *TeamTasksTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"action":  map[string]any{"type": "string", "enum": []string{"list", "create", "update", "complete"}, "description": "Action to perform"},
		"task_id": map[string]any{"type": "string", "description": "Task ID (required for update/complete)"},
		"title":   map[string]any{"type": "string", "description": "Task title (required for create)"},
		"status":  map[string]any{"type": "string", "description": "New status for update (e.g. in_progress, blocked)"},
		"result":  map[string]any{"type": "string", "description": "Result summary (for complete)"},
		"filter":  map[string]any{"type": "string", "description": "Status filter for list (e.g. pending, in_progress — omit for all)"},
	}, "required": []string{"action"}}
}

func (t *TeamTasksTool) Execute(ctx context.Context, args map[string]any) *Result {
	if t.backend == nil {
		return ErrorResult("team_tasks: task store not available")
	}
	action, _ := args["action"].(string)
	switch action {
	case "list":
		filter, _ := args["filter"].(string)
		rows, err := t.backend.ListAll(ctx, t.tenantID, filter, 50)
		if err != nil {
			return ErrorResult(fmt.Sprintf("team_tasks.list: %v", err))
		}
		if len(rows) == 0 {
			return TextResult("No tasks found.")
		}
		var sb strings.Builder
		for _, row := range rows {
			assignee := "(unassigned)"
			if row.AssignedTo != nil && *row.AssignedTo != "" {
				assignee = *row.AssignedTo
			}
			id := row.ID
			if len(id) > 8 {
				id = id[:8]
			}
			fmt.Fprintf(&sb, "- [%s] %s — %s (assignee: %s)\n", row.Status, id, row.Title, assignee)
		}
		return TextResult(sb.String())

	case "create":
		title, _ := args["title"].(string)
		if title == "" {
			return ErrorResult("team_tasks.create: title is required")
		}
		id, err := t.backend.CreateTask(ctx, t.tenantID, title)
		if err != nil {
			return ErrorResult(fmt.Sprintf("team_tasks.create: %v", err))
		}
		return TextResult(fmt.Sprintf("Task created: %s", id))

	case "update":
		taskID, _ := args["task_id"].(string)
		status, _ := args["status"].(string)
		if taskID == "" || status == "" {
			return ErrorResult("team_tasks.update: task_id and status are required")
		}
		if err := t.backend.Transition(ctx, taskID, status); err != nil {
			return ErrorResult(fmt.Sprintf("team_tasks.update: %v", err))
		}
		return TextResult(fmt.Sprintf("Task %s transitioned to %s.", taskID, status))

	case "complete":
		taskID, _ := args["task_id"].(string)
		result, _ := args["result"].(string)
		if taskID == "" {
			return ErrorResult("team_tasks.complete: task_id is required")
		}
		if err := t.backend.CompleteTask(ctx, taskID, result); err != nil {
			return ErrorResult(fmt.Sprintf("team_tasks.complete: %v", err))
		}
		return TextResult(fmt.Sprintf("Task %s marked complete.", taskID))

	default:
		return ErrorResult(fmt.Sprintf("team_tasks: unknown action %q", action))
	}
}

// --- team_message ---

// TeamMessageRuntime is the narrow interface this tool needs from RuntimeManager.
type TeamMessageRuntime interface {
	WakeAgentWithMessage(agentID, message string) bool
}

type TeamMessageTool struct {
	runtime TeamMessageRuntime
}

func NewTeamMessageTool(_ TeamTasksBackend, runtime TeamMessageRuntime) *TeamMessageTool {
	return &TeamMessageTool{runtime: runtime}
}
func (t *TeamMessageTool) Name() string { return "team_message" }
func (t *TeamMessageTool) Description() string {
	return "Send a message to a teammate agent (progress updates, questions, handoffs)."
}
func (t *TeamMessageTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"agent_id": map[string]any{"type": "string", "description": "Target agent ID (UUID)"},
		"text":     map[string]any{"type": "string", "description": "Message text"},
	}, "required": []string{"agent_id", "text"}}
}

func (t *TeamMessageTool) Execute(ctx context.Context, args map[string]any) *Result {
	agentID, _ := args["agent_id"].(string)
	text, _ := args["text"].(string)
	if agentID == "" || text == "" {
		return ErrorResult("team_message: agent_id and text are required")
	}
	if t.runtime == nil {
		return ErrorResult("team_message: runtime not available")
	}
	if !t.runtime.WakeAgentWithMessage(agentID, text) {
		return ErrorResult(fmt.Sprintf("team_message: agent %s is not running or unavailable", agentID))
	}
	return TextResult(fmt.Sprintf("Message sent to agent %s.", agentID))
}
