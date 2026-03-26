// AnyClaw - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 AnyClaw contributors

package agent

import (
	"bytes"
	"encoding/base64"
	"io"
	"os"
	"strings"

	"github.com/h2non/filetype"

	"github.com/anyclaw/anyclaw-server/pkg/logger"
	"github.com/anyclaw/anyclaw-server/pkg/media"
	"github.com/anyclaw/anyclaw-server/pkg/providers"
)

// resolveMediaRefs replaces media:// refs in message Media fields with base64 data URLs.
// Uses streaming base64 encoding (file handle ->encoder ->buffer) to avoid holding
// both raw bytes and encoded string in memory simultaneously.
// When maxImageEdge > 0, raster images may be downscaled before encoding to reduce payload size.
// Returns a new slice; original messages are not mutated.
func resolveMediaRefs(messages []providers.Message, store media.MediaStore, maxSize int, maxImageEdge int, jpegQuality int) []providers.Message {
	if store == nil {
		return messages
	}

	result := make([]providers.Message, len(messages))
	copy(result, messages)

	for i, m := range result {
		if len(m.Media) == 0 {
			continue
		}

		resolved := make([]string, 0, len(m.Media))
		for _, ref := range m.Media {
			if !strings.HasPrefix(ref, "media://") {
				resolved = append(resolved, ref)
				continue
			}
			if s, ok := encodeOneMediaRef(store, ref, maxSize, maxImageEdge, jpegQuality); ok {
				resolved = append(resolved, s)
			}
		}

		result[i].Media = resolved
	}

	return result
}

func encodeOneMediaRef(store media.MediaStore, ref string, maxSize int, maxImageEdge int, jpegQuality int) (string, bool) {
	localPath, meta, err := store.ResolveWithMeta(ref)
	if err != nil {
		logger.WarnCF("agent", "Failed to resolve media ref", map[string]any{
			"ref":   ref,
			"error": err.Error(),
		})
		return "", false
	}

	encodePath := localPath
	encodeMime := meta.ContentType
	var tmpShrink string

	if encodeMime == "" {
		kind, ftErr := filetype.MatchFile(localPath)
		if ftErr != nil || kind == filetype.Unknown {
			logger.WarnCF("agent", "Unknown media type, skipping", map[string]any{
				"path": localPath,
			})
			return "", false
		}
		encodeMime = kind.MIME.Value
	}

	if maxImageEdge > 0 {
		if p, mtyp, shrunk := shrinkImageFileIfNeeded(localPath, encodeMime, maxImageEdge, jpegQuality); shrunk {
			tmpShrink = p
			encodePath = p
			encodeMime = mtyp
		}
	}
	defer func() {
		if tmpShrink != "" {
			_ = os.Remove(tmpShrink)
		}
	}()

	info, err := os.Stat(encodePath)
	if err != nil {
		logger.WarnCF("agent", "Failed to stat media file", map[string]any{
			"path":  encodePath,
			"error": err.Error(),
		})
		return "", false
	}
	if info.Size() > int64(maxSize) {
		logger.WarnCF("agent", "Media file too large, skipping", map[string]any{
			"path":     encodePath,
			"size":     info.Size(),
			"max_size": maxSize,
		})
		return "", false
	}

	f, err := os.Open(encodePath)
	if err != nil {
		logger.WarnCF("agent", "Failed to open media file", map[string]any{
			"path":  encodePath,
			"error": err.Error(),
		})
		return "", false
	}

	prefix := "data:" + encodeMime + ";base64,"
	encodedLen := base64.StdEncoding.EncodedLen(int(info.Size()))
	var buf bytes.Buffer
	buf.Grow(len(prefix) + encodedLen)
	buf.WriteString(prefix)

	encoder := base64.NewEncoder(base64.StdEncoding, &buf)
	if _, err := io.Copy(encoder, f); err != nil {
		f.Close()
		logger.WarnCF("agent", "Failed to encode media file", map[string]any{
			"path":  encodePath,
			"error": err.Error(),
		})
		return "", false
	}
	encoder.Close()
	f.Close()

	return buf.String(), true
}
