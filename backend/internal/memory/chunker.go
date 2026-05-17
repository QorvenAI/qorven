// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package memory

import (
	"crypto/sha256"
	"fmt"
	"math"
	"strings"
)

// ContentHash returns a short SHA256 hex digest of the content (first 16 bytes).
func ContentHash(text string) string {
	h := sha256.Sum256([]byte(text))
	return fmt.Sprintf("%x", h[:16])
}

// TextChunk is a chunk of text with line number metadata.
type TextChunk struct {
	Text      string
	StartLine int
	EndLine   int
}

// ChunkWithOverlap splits content into overlapping chunks for better RAG retrieval.
// chunkSize is the target size in characters, overlap is the number of overlapping chars.
func ChunkWithOverlap(content string, chunkSize, overlap int) []string {
	if len(content) <= chunkSize {
		return []string{content}
	}
	if overlap >= chunkSize {
		overlap = chunkSize / 4
	}

	var chunks []string
	step := chunkSize - overlap
	for i := 0; i < len(content); i += step {
		end := i + chunkSize
		if end > len(content) {
			end = len(content)
		}
		chunk := strings.TrimSpace(content[i:end])
		if len(chunk) > 0 {
			chunks = append(chunks, chunk)
		}
		if end == len(content) {
			break
		}
	}
	return chunks
}

// ChunkTextWithLines splits text into chunks at paragraph boundaries with line metadata.
func ChunkTextWithLines(text string, maxChunkLen, overlap int) []TextChunk {
	if maxChunkLen <= 0 {
		maxChunkLen = 1000
	}
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= maxChunkLen/2 {
		overlap = maxChunkLen / 2
	}

	lines := strings.Split(text, "\n")
	var chunks []TextChunk
	var current strings.Builder
	startLine := 1
	var overlapLines []string
	overlapStartLine := 0

	flush := func(endLine int) {
		content := strings.TrimSpace(current.String())
		if content != "" {
			chunks = append(chunks, TextChunk{Text: content, StartLine: startLine, EndLine: endLine})
		}
		overlapLines = nil
		overlapStartLine = 0
		if overlap > 0 && endLine > 0 {
			charCount := 0
			for j := endLine - 1; j >= startLine-1 && j >= 0; j-- {
				lineLen := len(lines[j])
				if charCount+lineLen > overlap {
					break
				}
				charCount += lineLen + 1
				overlapLines = append(overlapLines, lines[j])
				overlapStartLine = j + 1
			}
			for left, right := 0, len(overlapLines)-1; left < right; left, right = left+1, right-1 {
				overlapLines[left], overlapLines[right] = overlapLines[right], overlapLines[left]
			}
		}
		current.Reset()
		if len(overlapLines) > 0 {
			startLine = overlapStartLine
			for k, ol := range overlapLines {
				if k > 0 {
					current.WriteString("\n")
				}
				current.WriteString(ol)
			}
		} else {
			startLine = endLine + 1
		}
	}

	for i, line := range lines {
		lineNum := i + 1
		if strings.TrimSpace(line) == "" && current.Len() > 0 && current.Len() >= maxChunkLen/2 {
			flush(lineNum - 1)
			continue
		}
		if current.Len() > 0 {
			current.WriteString("\n")
		}
		current.WriteString(line)
		if current.Len() >= maxChunkLen {
			flush(lineNum)
		}
	}
	if current.Len() > 0 {
		flush(len(lines))
	}
	return chunks
}

// CosineSimilarity computes the cosine similarity between two vectors.
// Returns a value between -1 and 1 (1 = identical).
func CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	return dot / denom
}
