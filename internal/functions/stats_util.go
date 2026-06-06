package functions

import (
	"strings"

	"autofilterbot/pkg/fileid"
	"github.com/PaulSonOfLars/gotgbot/v2"
)

// DetectLanguage returns the likely language of a query based on keywords.
func DetectLanguage(query string) string {
	query = strings.ToLower(query)
	
	keywords := map[string][]string{
		"Hindi":      {"hindi", "hin", "bollywood"},
		"Tamil":      {"tamil", "tam", "kollywood"},
		"Telugu":     {"telugu", "tel", "tollywood"},
		"Malayalam":  {"malayalam", "mal", "mollywood"},
		"Kannada":    {"kannada", "kan", "sandalwood"},
		"English":    {"english", "eng", "hollywood"},
		"Marathi":    {"marathi", "mar"},
		"Punjabi":    {"punjabi", "pun"},
		"Bengali":    {"bengali", "ben"},
		"Gujarati":   {"gujarati", "guj"},
	}

	for lang, keys := range keywords {
		for _, key := range keys {
			if strings.Contains(query, key) {
				return lang
			}
		}
	}

	return "Unknown"
}

// ExtractDC parses the Data Center ID from a Telegram file_id.
func ExtractDC(fileID string) int {
	if len(fileID) < 20 {
		return 0
	}
	fid, err := fileid.DecodeFileID(fileID)
	if err != nil {
		return 0
	}
	return fid.DC
}

// SetUserDC attempts to find the DC of a user from their profile photos and updates the user model.
func SetUserDC(bot *gotgbot.Bot, userId int64) int {
	photos, err := bot.GetUserProfilePhotos(userId, &gotgbot.GetUserProfilePhotosOpts{Limit: 1})
	if err != nil || len(photos.Photos) == 0 || len(photos.Photos[0]) == 0 {
		return 0
	}

	fileId := photos.Photos[0][0].FileId
	return ExtractDC(fileId)
}
