package functions

import (
	"errors"
	"regexp"
	"strings"

	"autofilterbot/internal/model"
	"github.com/PaulSonOfLars/gotgbot/v2"
)

var ErrFileNotFound = errors.New("no media was found in the message")

// HasVideoOrArchiveExtension returns true if the filename matches allowed video extensions.
func HasVideoOrArchiveExtension(fileName string) bool {
	fileName = strings.ToLower(fileName)
	allowedExts := []string{
		".mkv", ".mp4", ".avi", ".webm", ".mov", ".flv", ".wmv", ".3gp", ".m4v", ".ts", ".mpg", ".mpeg",
	}
	for _, ext := range allowedExts {
		if strings.HasSuffix(fileName, ext) {
			return true
		}
	}
	return false
}

var garbageRegex = regexp.MustCompile(`(?i)\b(sample|trailer|camrip|predvd|hdcam|telecine|hdtc|p-dvd|telesync|screener|dvdscr|scr|pre-dvd|hq-cam|hqcam|hc|tc|ts|cam)\b`)

// IsGarbageFile returns true if the filename contains garbage patterns like samples, subtitles, etc.
func IsGarbageFile(fileName string) bool {
	lower := strings.ToLower(fileName)
	if strings.HasSuffix(lower, ".srt") || strings.HasSuffix(lower, ".txt") || strings.HasSuffix(lower, ".nfo") || strings.HasSuffix(lower, ".idx") || strings.HasSuffix(lower, ".sub") {
		return true
	}
	return garbageRegex.MatchString(lower)
}

// FileFromMessage extracts data about a file from the message.
func FileFromMessage(m *gotgbot.Message) *model.File {
	if m == nil {
		return nil
	}

	var (
		fileSize                             int64
		fileId, uniqueId, fileName, fileType string
	)

	switch {
	case m.Document != nil:
		fileId = m.Document.FileId
		uniqueId = m.Document.FileUniqueId
		fileName = m.Document.FileName
		fileSize = m.Document.FileSize
		fileType = model.FileTypeDocument
	case m.Video != nil:
		fileId = m.Video.FileId
		uniqueId = m.Video.FileUniqueId
		fileName = m.Video.FileName
		fileSize = m.Video.FileSize
		fileType = model.FileTypeVideo
	default:
		return nil
	}

	// Filter out non-video and non-archive documents, or garbage files
	if IsGarbageFile(fileName) || !HasVideoOrArchiveExtension(fileName) {
		return nil
	}

	fileName = RemoveSymbols(CleanPromoFromName(RemoveExtension(fileName)))

	return &model.File{
		UniqueId:    uniqueId,
		FileId:      fileId,
		FileName:    fileName,
		FileType:    fileType,
		FileSize:    fileSize,
		Time:        m.Date,
		ChatId:      m.Chat.Id,
		MessageLink: m.GetLink(),
	}
}
