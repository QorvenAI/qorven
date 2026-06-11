// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

// Package editapply implements a reliable search-and-replace engine for
// autonomous code editing.  It mirrors the 4-strategy cascade used by Aider:
// exact → whitespace-flexible → skip-leading-blank → ellipsis.  The first
// strategy that produces a unique match wins.  On total failure, the error
// includes a "did you mean" hint drawn from the closest-matching window of
// file lines so the calling agent can self-repair.
package editapply

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ── Sentinel errors ──────────────────────────────────────────────────────────

// ErrNoMatch is returned when none of the four strategies can locate the
// search block inside the file content.
var ErrNoMatch = errors.New("editapply: search block not found in file")

// ErrNotUnique is returned when a strategy finds more than one occurrence of
// the search block.  Ambiguous replacement is refused by design.
var ErrNotUnique = errors.New("editapply: search block matches more than one location")

// notUniqueErr wraps ErrNotUnique so callers can use errors.Is.
type notUniqueErr struct{ msg string }

func (e *notUniqueErr) Error() string { return e.msg }
func (e *notUniqueErr) Is(target error) bool {
	return target == ErrNotUnique
}
func (e *notUniqueErr) Unwrap() error { return ErrNotUnique }

// noMatchErr wraps ErrNoMatch and carries the did-you-mean hint.
type noMatchErr struct{ msg string }

func (e *noMatchErr) Error() string { return e.msg }
func (e *noMatchErr) Is(target error) bool {
	return target == ErrNoMatch
}
func (e *noMatchErr) Unwrap() error { return ErrNoMatch }

// similarHintMarker is the stable substring embedded in every no-match error
// that ContainsSimilar looks for.
const similarHintMarker = "did you mean"

// IsNotUnique reports whether err (or any error in its chain) is ErrNotUnique.
func IsNotUnique(err error) bool { return errors.Is(err, ErrNotUnique) }

// ContainsSimilar reports whether the error string includes the did-you-mean
// hint inserted by the no-match error path.
func ContainsSimilar(s string) bool { return strings.Contains(s, similarHintMarker) }

// ── Public entry point ───────────────────────────────────────────────────────

// Apply locates search inside content using the 4-strategy cascade and returns
// a new string with the matched region replaced by replace.
//
// Strategies tried in order:
//  1. Exact verbatim match (must appear exactly once).
//  2. Whitespace-flexible: compare lines ignoring leading-indent differences.
//  3. Skip-leading-blank: drop a single spurious leading blank from search,
//     then retry strategies 1–2.
//  4. Ellipsis: lines matching `^\s*\.\.\.\s*$` in search act as "keep
//     unchanged" placeholders; split on them and match/replace chunk-wise.
//
// On failure returns a noMatchErr wrapping ErrNoMatch with a similarity hint,
// or a notUniqueErr wrapping ErrNotUnique.
func Apply(content, search, replace string) (string, error) {
	// ── Strategy 1: exact ────────────────────────────────────────────────────
	if result, err := applyExact(content, search, replace); err == nil {
		return result, nil
	} else if IsNotUnique(err) {
		return "", err
	}

	// ── Strategy 2: whitespace-flexible ─────────────────────────────────────
	if result, err := applyWhitespaceFlex(content, search, replace); err == nil {
		return result, nil
	} else if IsNotUnique(err) {
		return "", err
	}

	// ── Strategy 3: skip one spurious leading blank, then retry 1+2 ─────────
	if stripped, ok := dropLeadingBlank(search); ok {
		if result, err := applyExact(content, stripped, replace); err == nil {
			return result, nil
		} else if IsNotUnique(err) {
			return "", err
		}
		if result, err := applyWhitespaceFlex(content, stripped, replace); err == nil {
			return result, nil
		} else if IsNotUnique(err) {
			return "", err
		}
	}

	// ── Strategy 4: ellipsis ─────────────────────────────────────────────────
	if containsEllipsis(search) {
		if result, err := applyEllipsis(content, search, replace); err == nil {
			return result, nil
		} else if IsNotUnique(err) {
			return "", err
		}
	}

	// ── Total failure: build hint ─────────────────────────────────────────────
	hint := findSimilarLines(content, search)
	msg := fmt.Sprintf("%v\n%s:\n%s", ErrNoMatch, similarHintMarker, hint)
	return "", &noMatchErr{msg: msg}
}

// ── Strategy 1: exact ────────────────────────────────────────────────────────

func applyExact(content, search, replace string) (string, error) {
	count := strings.Count(content, search)
	switch count {
	case 0:
		return "", ErrNoMatch
	case 1:
		return strings.Replace(content, search, replace, 1), nil
	default:
		return "", &notUniqueErr{
			msg: fmt.Sprintf("%v: found %d occurrences of search block", ErrNotUnique, count),
		}
	}
}

// ── Strategy 2: whitespace-flexible ─────────────────────────────────────────
//
// Algorithm:
//  1. Split both file and search into lines.
//  2. Compute the minimum leading whitespace of the search lines (ignoring
//     blank lines) → searchIndent.
//  3. Slide a window of len(searchLines) over the file lines; for each
//     position, check that each search line (stripped of searchIndent) equals
//     the corresponding file line stripped of its own leading whitespace, and
//     that the per-line indent difference is uniform across the window
//     (fileIndent - searchIndent == offset, same for all non-blank lines).
//  4. On a match, reconstruct the replacement lines by replacing each search
//     line's stripped content with the replace line's stripped content, but
//     applying the file's actual per-line indentation.  The replace block's
//     own indent structure (relative to its minimum indent) is preserved and
//     then shifted by the same offset that was found in step 3.

func applyWhitespaceFlex(content, search, replace string) (string, error) {
	fileLines := splitLines(content)
	searchLines := splitLines(search)
	replaceLines := splitLines(replace)
	n := len(searchLines)
	if n == 0 {
		return "", ErrNoMatch
	}

	searchMinIndent := minIndent(searchLines)
	replaceMinIndent := minIndent(replaceLines)

	matchIdx := -1
	for i := 0; i <= len(fileLines)-n; i++ {
		if windowMatches(fileLines[i:i+n], searchLines, searchMinIndent) {
			if matchIdx != -1 {
				return "", &notUniqueErr{
					msg: fmt.Sprintf("%v: whitespace-flexible found multiple matching windows", ErrNotUnique),
				}
			}
			matchIdx = i
		}
	}
	if matchIdx == -1 {
		return "", ErrNoMatch
	}

	// Determine the indent offset from the first non-blank search line.
	offset := indentOffset(fileLines[matchIdx:matchIdx+n], searchLines, searchMinIndent)

	// Build replacement lines with adjusted indentation.
	adjusted := adjustIndent(replaceLines, replaceMinIndent, offset)

	// Splice: fileLines[:matchIdx] + adjusted + fileLines[matchIdx+n:]
	out := make([]string, 0, len(fileLines)-n+len(adjusted))
	out = append(out, fileLines[:matchIdx]...)
	out = append(out, adjusted...)
	out = append(out, fileLines[matchIdx+n:]...)
	return joinLines(out), nil
}

// windowMatches returns true if every line in window (file) matches the
// corresponding search line when both have ALL leading whitespace stripped for
// content comparison, and the per-line indent difference (fileIndent -
// searchIndent) is uniform across all non-blank lines.
func windowMatches(window, search []string, _ int) bool {
	if len(window) != len(search) {
		return false
	}
	offset := -2 // -2 means "unset"; -1 could be a valid delta
	for i, sl := range search {
		fl := window[i]
		// Blank lines in search match blank lines in the file only.
		if strings.TrimSpace(sl) == "" {
			if strings.TrimSpace(fl) != "" {
				return false
			}
			continue
		}
		// Compare bare content (no leading whitespace).
		searchContent := lineContent(sl)
		fileContent := lineContent(fl)
		if fileContent != searchContent {
			return false
		}
		// Check the indent delta is uniform.
		delta := leadingWhitespace(fl) - leadingWhitespace(sl)
		if offset == -2 {
			offset = delta
		} else if delta != offset {
			return false
		}
	}
	return true
}

// indentOffset returns the per-line indent delta (file indent − search indent)
// from the first non-blank search line in the window.
func indentOffset(window, search []string, _ int) int {
	for i, sl := range search {
		if strings.TrimSpace(sl) != "" {
			return leadingWhitespace(window[i]) - leadingWhitespace(sl)
		}
	}
	return 0
}

// adjustIndent rebuilds replaceLines preserving the replace block's internal
// relative indentation and shifting the whole block by offset (the delta
// found between file and search indents).  The indentation character used by
// the file for the first non-blank line of the matched region is replicated.
func adjustIndent(lines []string, _ int, offset int) []string {
	result := make([]string, len(lines))
	for i, l := range lines {
		if strings.TrimSpace(l) == "" {
			result[i] = l // preserve blank lines as-is
			continue
		}
		searchIndent := leadingWhitespace(l)
		content := lineContent(l)
		newIndent := searchIndent + offset
		if newIndent < 0 {
			newIndent = 0
		}
		nl := lineTerminator(l)
		result[i] = strings.Repeat(" ", newIndent) + content + nl
	}
	return result
}

// lineContent returns a line's text with all leading whitespace and trailing
// line-ending characters removed.
func lineContent(s string) string {
	return strings.TrimRight(strings.TrimLeft(s, " \t"), "\r\n")
}

// lineTerminator returns the trailing line-ending characters ("\r\n", "\n", or "").
func lineTerminator(s string) string {
	if strings.HasSuffix(s, "\r\n") {
		return "\r\n"
	}
	if strings.HasSuffix(s, "\n") {
		return "\n"
	}
	return ""
}

// ── Strategy 3: skip leading blank ──────────────────────────────────────────

// dropLeadingBlank returns (search without its first line, true) when the
// first line of search is blank (empty or only whitespace).
func dropLeadingBlank(search string) (string, bool) {
	idx := strings.Index(search, "\n")
	if idx == -1 {
		// single-line search: check if it's blank
		if strings.TrimSpace(search) == "" {
			return "", true
		}
		return "", false
	}
	first := search[:idx]
	if strings.TrimSpace(first) == "" {
		return search[idx+1:], true
	}
	return "", false
}

// ── Strategy 4: ellipsis ─────────────────────────────────────────────────────
//
// Lines matching `^\s*\.\.\.\s*$` in the search block act as "keep unchanged"
// placeholders.  The search (and corresponding replace) are split on these
// lines into alternating anchor chunks.  Each anchor chunk is located in the
// file exactly once (in order, non-overlapping).  The spans between anchors
// are preserved verbatim.
//
// Replacement indentation: each replace anchor chunk undergoes the same
// whitespace-flex indent adjustment as strategy 2 (the offset is determined
// by comparing the matched file window with the search anchor chunk).

var ellipsisRE = regexp.MustCompile(`(?m)^\s*\.\.\.\s*$`)

func containsEllipsis(s string) bool { return ellipsisRE.MatchString(s) }

// splitOnEllipsis splits a block of lines on ellipsis lines.
// It returns the list of non-ellipsis chunks (each a []string of lines) and
// an indicator of whether the block starts/ends with an ellipsis.
func splitOnEllipsis(lines []string) [][]string {
	var chunks [][]string
	current := []string{}
	for _, l := range lines {
		if ellipsisRE.MatchString(l) {
			chunks = append(chunks, current)
			current = []string{}
		} else {
			current = append(current, l)
		}
	}
	chunks = append(chunks, current)
	return chunks
}

func applyEllipsis(content, search, replace string) (string, error) {
	fileLines := splitLines(content)
	searchLines := splitLines(search)
	replaceLines := splitLines(replace)

	searchChunks := splitOnEllipsis(searchLines)
	replaceChunks := splitOnEllipsis(replaceLines)

	// The number of chunks must match (same number of ellipsis lines in both).
	if len(searchChunks) != len(replaceChunks) {
		return "", fmt.Errorf("%w: search has %d chunks, replace has %d",
			ErrNoMatch, len(searchChunks), len(replaceChunks))
	}

	// Locate each search chunk in the file, in order, non-overlapping.
	type span struct{ start, end int } // [start, end) in fileLines
	spans := make([]span, len(searchChunks))
	cursor := 0
	for ci, chunk := range searchChunks {
		if len(chunk) == 0 {
			// Empty chunk (ellipsis at start/end or adjacent ellipses):
			// anchor at cursor with zero width.
			spans[ci] = span{cursor, cursor}
			continue
		}
		idx, cnt := findChunkInLines(fileLines, chunk, cursor)
		if cnt == 0 {
			return "", ErrNoMatch
		}
		if cnt > 1 {
			return "", &notUniqueErr{
				msg: fmt.Sprintf("%v: ellipsis chunk %d matches %d locations", ErrNotUnique, ci, cnt),
			}
		}
		spans[ci] = span{idx, idx + len(chunk)}
		cursor = idx + len(chunk)
	}

	// Assemble the output:
	// For each chunk i: output replaceChunks[i] (indent-adjusted), then
	// preserve the file lines between spans[i].end and spans[i+1].start.
	var out []string
	for ci, rChunk := range replaceChunks {
		// Adjust indentation of the replace chunk to match the file region.
		var adjusted []string
		if len(rChunk) > 0 {
			sChunk := searchChunks[ci]
			searchMin := minIndent(sChunk)
			replaceMin := minIndent(rChunk)
			offset := 0
			if len(sChunk) > 0 {
				offset = indentOffset(fileLines[spans[ci].start:spans[ci].end], sChunk, searchMin)
			}
			adjusted = adjustIndent(rChunk, replaceMin, offset)
		}
		out = append(out, adjusted...)

		// Append the "kept" lines between this span's end and the next span's start.
		if ci < len(spans)-1 {
			kept := fileLines[spans[ci].end:spans[ci+1].start]
			out = append(out, kept...)
		}
	}

	// Append any trailing file lines after the last span.
	if len(spans) > 0 {
		last := spans[len(spans)-1]
		out = append(out, fileLines[last.end:]...)
	}

	// Prepend any file lines before the first span.
	prefix := fileLines[:spans[0].start]
	result := make([]string, 0, len(prefix)+len(out))
	result = append(result, prefix...)
	result = append(result, out...)
	return joinLines(result), nil
}

// findChunkInLines looks for chunk (a slice of lines) inside haystack starting
// at fromIdx.  Returns the index of the first match and the total number of
// matches found anywhere in haystack[fromIdx:].
func findChunkInLines(haystack, chunk []string, fromIdx int) (firstIdx, count int) {
	firstIdx = -1
	n := len(chunk)
	for i := fromIdx; i <= len(haystack)-n; i++ {
		if linesEqual(haystack[i:i+n], chunk) {
			count++
			if firstIdx == -1 {
				firstIdx = i
			}
		}
	}
	return firstIdx, count
}

// linesEqual compares two equal-length slices of strings.
func linesEqual(a, b []string) bool {
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ── Similarity hint ──────────────────────────────────────────────────────────
//
// findSimilarLines finds the window of file lines whose content is most similar
// to the search block, using a simple character-level overlap ratio
// (2 * common_chars / (len_a + len_b)).  If the best similarity is ≥ 0.6,
// the window is included in the error hint.

const similarityThreshold = 0.6

func findSimilarLines(content, search string) string {
	fileLines := splitLines(content)
	searchLines := splitLines(search)
	n := len(searchLines)
	if n == 0 || len(fileLines) == 0 {
		return "(no candidate lines found)"
	}
	if n > len(fileLines) {
		n = len(fileLines)
	}

	searchFlat := strings.Join(searchLines, "\n")
	bestScore := 0.0
	bestWindow := ""

	for i := 0; i <= len(fileLines)-n; i++ {
		window := strings.Join(fileLines[i:i+n], "\n")
		score := similarityRatio(window, searchFlat)
		if score > bestScore {
			bestScore = score
			bestWindow = window
		}
	}

	if bestScore < similarityThreshold {
		return "(no similar lines found above threshold)"
	}
	return bestWindow
}

// similarityRatio computes 2 * |common characters| / (len(a) + len(b)).
// This is the Sørensen–Dice coefficient over character bigrams, approximated
// here as simple character frequency overlap for performance.
func similarityRatio(a, b string) float64 {
	if len(a)+len(b) == 0 {
		return 0
	}
	// Build character frequency maps.
	fa := charFreq(a)
	fb := charFreq(b)
	var common int
	for ch, ca := range fa {
		if cb, ok := fb[ch]; ok {
			if ca < cb {
				common += ca
			} else {
				common += cb
			}
		}
	}
	return float64(2*common) / float64(len(a)+len(b))
}

func charFreq(s string) map[rune]int {
	m := make(map[rune]int, len(s))
	for _, r := range s {
		m[r]++
	}
	return m
}

// ── Line utilities ───────────────────────────────────────────────────────────

// splitLines splits s into individual lines, preserving the line terminator
// as part of each element.  The final element may lack a terminator if the
// input does not end with one.
//
// We split on "\n" and re-append "\n" to every element except the last (if
// the original ended with "\n", the last split element is empty and is kept
// as an empty sentinel).  This ensures round-trip fidelity.
func splitLines(s string) []string {
	if s == "" {
		return []string{""}
	}
	parts := strings.Split(s, "\n")
	lines := make([]string, len(parts))
	for i, p := range parts {
		if i < len(parts)-1 {
			lines[i] = p + "\n"
		} else {
			lines[i] = p // last part (may be "" if s ends with \n)
		}
	}
	return lines
}

// joinLines is the inverse of splitLines.
func joinLines(lines []string) string {
	return strings.Join(lines, "")
}

// leadingWhitespace returns the number of leading space/tab characters in s.
// For simplicity, tabs count as 1 (consistent with the comparison logic).
func leadingWhitespace(s string) int {
	// Strip the trailing newline for measurement purposes.
	raw := strings.TrimRight(s, "\r\n")
	for i, ch := range raw {
		if ch != ' ' && ch != '\t' {
			return i
		}
	}
	return len(raw) // fully blank line
}

// minIndent returns the minimum leading-whitespace count across all non-blank
// lines in the slice.
func minIndent(lines []string) int {
	min := -1
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		ind := leadingWhitespace(l)
		if min == -1 || ind < min {
			min = ind
		}
	}
	if min == -1 {
		return 0
	}
	return min
}

// stripPrefix removes exactly n leading bytes (spaces/tabs) from s.
// If s has fewer than n leading whitespace chars, it returns s with all
// leading whitespace removed.
func stripPrefix(s string, n int) string {
	// Trim trailing newline, strip n bytes, re-add newline.
	nl := ""
	raw := s
	if strings.HasSuffix(raw, "\n") {
		nl = "\n"
		raw = raw[:len(raw)-1]
	}
	if strings.HasSuffix(raw, "\r") {
		nl = "\r\n"
		raw = raw[:len(raw)-1]
	}
	count := 0
	for count < n && count < len(raw) {
		if raw[count] == ' ' || raw[count] == '\t' {
			count++
		} else {
			break
		}
	}
	return raw[count:] + nl
}
