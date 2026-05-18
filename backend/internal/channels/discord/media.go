// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package discord

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

// Media pipeline: resolve attachments, download, classify, send.

const maxMediaBytes = 10 * 1024 * 1024 // Discord default upload limit (changed Jan 2025: 25MB → 10MB)

type mediaItem struct {
	Name     string
	MimeType string
	Size     int
	URL      string
	Category string // image, audio, video, document, text, file
	Content  string // extracted text for text files
}

// resolveMedia extracts all attachments from a Discord message
func resolveMedia(attachments []*discordgo.MessageAttachment) ([]mediaItem, string) {
	var items []mediaItem
	var extraContent string

	for _, att := range attachments {
		item := mediaItem{
			Name:     att.Filename,
			MimeType: att.ContentType,
			Size:     att.Size,
			URL:      att.URL,
			Category: classifyMediaType(att.ContentType),
		}
		items = append(items, item)
		extraContent += buildDiscordMediaTag(item)
	}
	return items, extraContent
}

func classifyMediaType(mime string) string {
	switch {
	case strings.HasPrefix(mime, "image/"): return "image"
	case strings.HasPrefix(mime, "audio/"): return "audio"
	case strings.HasPrefix(mime, "video/"): return "video"
	case strings.HasPrefix(mime, "text/"): return "text"
	case mime == "application/pdf": return "document"
	case mime == "application/json" || mime == "application/xml": return "text"
	default: return "file"
	}
}

func buildDiscordMediaTag(item mediaItem) string {
	switch item.Category {
	case "image":
		return fmt.Sprintf("\n<attached_file name=\"%s\" type=\"%s\">\n[Image: %d bytes]\n</attached_file>", item.Name, item.MimeType, item.Size)
	case "text":
		if item.Content != "" {
			return fmt.Sprintf("\n<attached_file name=\"%s\" type=\"%s\">\n%s\n</attached_file>", item.Name, item.MimeType, item.Content)
		}
		return fmt.Sprintf("\n[File: %s (%s, %d bytes)]", item.Name, item.MimeType, item.Size)
	default:
		return fmt.Sprintf("\n[%s: %s (%s, %d bytes)]", item.Category, item.Name, item.MimeType, item.Size)
	}
}

// downloadFromURL downloads a file from a URL to a temp path
func downloadFromURL(fileURL string) (string, error) {
	os.MkdirAll("/tmp/qorven-discord", 0755)
	parsed, _ := url.Parse(fileURL)
	ext := filepath.Ext(parsed.Path)
	if ext == "" { ext = ".bin" }

	tmpFile, err := os.CreateTemp("/tmp/qorven-discord", "dc-*"+ext)
	if err != nil { return "", err }
	defer tmpFile.Close()

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(fileURL)
	if err != nil { os.Remove(tmpFile.Name()); return "", err }
	defer resp.Body.Close()

	written, err := io.Copy(tmpFile, io.LimitReader(resp.Body, maxMediaBytes))
	if err != nil { os.Remove(tmpFile.Name()); return "", err }

	slog.Info("discord.media.downloaded", "url", fileURL[:min(len(fileURL), 60)], "bytes", written)
	return tmpFile.Name(), nil
}

func urlFileName(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil { return "file" }
	return filepath.Base(parsed.Path)
}

// sendMediaMessage sends a file as a Discord message attachment
func (d *DiscordChannel) sendMediaMessage(channelID, filePath, caption string) error {
	f, err := os.Open(filePath)
	if err != nil { return err }
	defer f.Close()

	_, err = d.session.ChannelFileSendWithMessage(channelID, caption, filepath.Base(filePath), f)
	return err
}

// SendImage sends an image from URL (downloads first, then uploads)
func (d *DiscordChannel) SendImage(channelID, imageURL, caption string) error {
	localPath, err := downloadFromURL(imageURL)
	if err != nil { return d.sendFallbackLink(channelID, imageURL, caption) }
	defer os.Remove(localPath)
	return d.sendMediaMessage(channelID, localPath, caption)
}

func (d *DiscordChannel) sendFallbackLink(channelID, url, caption string) error {
	text := caption
	if text != "" { text += "\n" }
	text += url
	_, err := d.session.ChannelMessageSend(channelID, text)
	return err
}
