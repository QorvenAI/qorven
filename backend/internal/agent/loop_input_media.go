// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"log/slog"

	"github.com/qorvenai/qorven/internal/providers"
)

// maxMediaReloadMessages limits how many historical messages to reload media for.
const maxMediaReloadMessages = 10

// CollectRefsByKind gathers MediaRefs of a given kind from message history
// (reverse order) and current-turn refs. Historical first, current last.
// Note: Requires MediaRefs field on providers.Message (add if needed).
func CollectRefsByKind(messages []providers.Message, currentRefs []MediaRef, kind string) []MediaRef {
	var refs []MediaRef
	// Historical refs would come from messages[i].MediaRefs if that field exists
	// For now, just return current refs filtered by kind
	for _, ref := range currentRefs {
		if ref.Kind == kind {
			refs = append(refs, ref)
		}
	}
	return refs
}

// MediaRef is defined in media_handling.go

// MediaInput represents incoming media from a request.
type MediaInput struct {
	Kind     string // "image", "document", "audio", "video"
	Path     string
	Base64   string
	MimeType string
	Name     string
}

// EnrichInputMedia processes incoming media (images, documents, audio, video),
// persists them, enriches messages with media tags, and populates context
// with refs for tool access.
func EnrichInputMedia(ctx context.Context, media []MediaInput, messages []providers.Message, workspace string) (context.Context, []providers.Message, []MediaRef) {
	if len(media) == 0 {
		return ctx, messages, nil
	}

	var mediaRefs []MediaRef
	var imageFiles []string

	for _, m := range media {
		ref := MediaRef{
			Kind:     m.Kind,
			Path:     m.Path,
			MimeType: m.MimeType,
		}
		mediaRefs = append(mediaRefs, ref)

		if m.Kind == "image" && m.Path != "" {
			imageFiles = append(imageFiles, m.Path)
		}
	}

	// Load images into context for vision tools
	if len(imageFiles) > 0 {
		ctx = WithMediaImages(ctx, imageFiles)
		slog.Info("vision: loaded images into context", "count", len(imageFiles))
	}

	// Collect document refs for read_document tool
	if docRefs := filterRefsByKind(mediaRefs, "document"); len(docRefs) > 0 {
		ctx = WithMediaDocRefs(ctx, docRefs)
	}

	// Collect audio refs for read_audio tool
	if audioRefs := filterRefsByKind(mediaRefs, "audio"); len(audioRefs) > 0 {
		ctx = WithMediaAudioRefs(ctx, audioRefs)
	}

	// Collect video refs for read_video tool
	if videoRefs := filterRefsByKind(mediaRefs, "video"); len(videoRefs) > 0 {
		ctx = WithMediaVideoRefs(ctx, videoRefs)
	}

	return ctx, messages, mediaRefs
}

func filterRefsByKind(refs []MediaRef, kind string) []MediaRef {
	var filtered []MediaRef
	for _, ref := range refs {
		if ref.Kind == kind {
			filtered = append(filtered, ref)
		}
	}
	return filtered
}

// Context keys for media
type mediaImagesKey struct{}
type mediaDocRefsKey struct{}
type mediaAudioRefsKey struct{}
type mediaVideoRefsKey struct{}

// WithMediaImages adds image paths to context.
func WithMediaImages(ctx context.Context, images []string) context.Context {
	return context.WithValue(ctx, mediaImagesKey{}, images)
}

// MediaImagesFromCtx retrieves image paths from context.
func MediaImagesFromCtx(ctx context.Context) []string {
	if v := ctx.Value(mediaImagesKey{}); v != nil {
		return v.([]string)
	}
	return nil
}

// WithMediaDocRefs adds document refs to context.
func WithMediaDocRefs(ctx context.Context, refs []MediaRef) context.Context {
	return context.WithValue(ctx, mediaDocRefsKey{}, refs)
}

// MediaDocRefsFromCtx retrieves document refs from context.
func MediaDocRefsFromCtx(ctx context.Context) []MediaRef {
	if v := ctx.Value(mediaDocRefsKey{}); v != nil {
		return v.([]MediaRef)
	}
	return nil
}

// WithMediaAudioRefs adds audio refs to context.
func WithMediaAudioRefs(ctx context.Context, refs []MediaRef) context.Context {
	return context.WithValue(ctx, mediaAudioRefsKey{}, refs)
}

// MediaAudioRefsFromCtx retrieves audio refs from context.
func MediaAudioRefsFromCtx(ctx context.Context) []MediaRef {
	if v := ctx.Value(mediaAudioRefsKey{}); v != nil {
		return v.([]MediaRef)
	}
	return nil
}

// WithMediaVideoRefs adds video refs to context.
func WithMediaVideoRefs(ctx context.Context, refs []MediaRef) context.Context {
	return context.WithValue(ctx, mediaVideoRefsKey{}, refs)
}

// MediaVideoRefsFromCtx retrieves video refs from context.
func MediaVideoRefsFromCtx(ctx context.Context) []MediaRef {
	if v := ctx.Value(mediaVideoRefsKey{}); v != nil {
		return v.([]MediaRef)
	}
	return nil
}
