package autofilter

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"autofilterbot/internal/functions"
	"autofilterbot/internal/model"
	"autofilterbot/pkg/shortener"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

var promoRegex = regexp.MustCompile(`(?i)(?:\[\s*@\w+\s*\]|\(\s*@\w+\s*\)|@\w+)`)

type Files []File

// File wraps model.File with some search result specific data.
type File struct {
	model.File
	// Indicates whether the file was selected from the selection menu.
	IsSelected bool `json:"selected,omitempty"`
}

// Process returns a slice of buttons to be used in message markup.
func (files Files) Process(chatId int64, botUsername string, opts ProcessFilesOptions) [][]gotgbot.InlineKeyboardButton {
	return ProcessFiles(files, chatId, botUsername, opts)
}

type ProcessFilesOptions interface {
	GetButtonTemplate() string
	GetSizeButton() bool
	GetShortener() shortener.Shortener
}

// ProcessFiles changes files into a keyboard slice to be used as markup in a message.
func ProcessFiles(files Files, chatId int64, botUsername string, opts ProcessFilesOptions) [][]gotgbot.InlineKeyboardButton {
	var (
		hasShortener = opts.GetShortener().ApiKey != ""
		result       = make([][]gotgbot.InlineKeyboardButton, 0, len(files))
	)

	for _, f := range files {
		// Filter out samples, trailers, srt, etc.
		if IsGarbageFile(f.FileName) {
			continue
		}

		btnText := FormatFileButtonText(f.FileName, f.FileSize)
		var btn gotgbot.InlineKeyboardButton

		if chatId > 0 {
			// Private chat: Send directly via callback
			btn = gotgbot.InlineKeyboardButton{Text: btnText, CallbackData: "sendfile|" + f.UniqueId}
		} else {
			// Group or unknown: Use start URL
			url := fmt.Sprintf("https://t.me/%s?start=%s", botUsername, URLData{
				FileUniqueId: f.UniqueId,
				ChatId:       chatId,
				HasShortener: hasShortener,
			}.Encode())
			btn = gotgbot.InlineKeyboardButton{Text: btnText, Url: url}
		}

		result = append(result, []gotgbot.InlineKeyboardButton{btn})
	}

	return result
}

// FormatFileButtonText formats file details nicely with minimal emoji use.
func FormatFileButtonText(fileName string, fileSize int64) string {
	// 1. Get size string
	sizeStr := functions.FileSizeToString(fileSize)

	// 2. Clean promotional/credit text first
	cleanedName := functions.CleanPromoFromName(fileName)

	// 3. Remove file extension
	if idx := strings.LastIndex(cleanedName, "."); idx != -1 {
		ext := cleanedName[idx:]
		if len(ext) <= 5 {
			cleanedName = cleanedName[:idx]
		}
	}

	lowerName := strings.ToLower(cleanedName)

	// 4. Extract year
	year := ""
	yearRegex := regexp.MustCompile(`\b(19|20)\d{2}\b`)
	if match := yearRegex.FindString(cleanedName); match != "" {
		year = match
	}

	// 5. Extract series metadata (Season & Episode)
	isSeries := IsSeriesFile(cleanedName)
	s, e := ExtractSeriesMetadata(cleanedName)

	// 6. Extract language
	var langs []string
	langMap := map[string][]string{
		"Hindi":     {"hindi", "hin"},
		"English":   {"english", "eng"},
		"Tamil":     {"tamil", "tam"},
		"Telugu":    {"telugu", "tel"},
		"Malayalam": {"malayalam", "mal"},
		"Kannada":   {"kannada", "kan"},
		"Bengali":   {"bengali", "ben"},
		"Marathi":   {"marathi", "mar"},
		"Bhojpuri":  {"bhojpuri"},
		"Punjabi":   {"punjabi", "pun"},
		"Gujarati":  {"gujarati", "guj"},
		"Multi":     {"multi", "dual", "mux", "dubbed", "dub"},
	}

	isMulti := false
	for _, tag := range langMap["Multi"] {
		if strings.Contains(lowerName, tag) {
			isMulti = true
			break
		}
	}

	for name, tags := range langMap {
		if name == "Multi" {
			continue
		}
		for _, tag := range tags {
			if strings.Contains(lowerName, tag) {
				langs = append(langs, name)
				break
			}
		}
	}

	langStr := ""
	if isMulti {
		langStr = "Multi"
	} else if len(langs) > 0 {
		langStr = strings.Join(langs, "-")
	}

	// 7. Extract quality
	quality := ""
	resRegex := regexp.MustCompile(`(?i)(2160p|1080p|720p|480p|360p|4k)`)
	if match := resRegex.FindString(cleanedName); match != "" {
		quality = strings.ToUpper(match)
	}

	sourceRegex := regexp.MustCompile(`(?i)(bluray|web-dl|webrip|hdtv|camrip|brrip|dvdrip|hdrip|tc|ts)`)
	if match := sourceRegex.FindString(cleanedName); match != "" {
		src := strings.ToUpper(match)
		if quality != "" {
			quality = quality + " " + src
		} else {
			quality = src
		}
	}

	// Extract version/edition
	version := ""
	versionRegex := regexp.MustCompile(`(?i)\b(color|bw|b&w|black\s+and\s+white|imax|remastered|extended|unrated|directors?\s+cut|dc|special\s+edition|se|proper|repack)\b`)
	if match := versionRegex.FindString(cleanedName); match != "" {
		vUpper := strings.ToUpper(match)
		switch vUpper {
		case "BW", "B&W", "IMAX", "DC", "SE":
			version = vUpper
		default:
			version = strings.Title(strings.ToLower(match))
		}
	}

	// 8. Extract clean title
	title := ExtractBaseTitle(fileName)
	bracketRegex := regexp.MustCompile(`^(?i)(?:\[[^\]]+\]|\([^\)]+\))\s*`)
	for {
		loc := bracketRegex.FindString(title)
		if loc == "" {
			break
		}
		title = strings.TrimSpace(title[len(loc):])
	}

	if title == "" {
		title = ExtractBaseTitle(fileName)
	}

	// Strip year from title if it exists at the end
	if year != "" {
		yearPatterns := []string{
			" (" + year + ")",
			" [" + year + "]",
			" " + year,
		}
		for _, yp := range yearPatterns {
			if strings.HasSuffix(title, yp) {
				title = strings.TrimSpace(title[:len(title)-len(yp)])
				break
			}
		}
	}

	if len(title) > 25 {
		title = title[:22] + "..."
	}

	// 9. Format button string
	var info []string
	if langStr != "" {
		info = append(info, langStr)
	}
	if quality != "" {
		info = append(info, quality)
	}
	if version != "" {
		info = append(info, version)
	}

	infoStr := strings.Join(info, " • ")

	var btnText string
	if isSeries {
		seriesParts := ""
		rangeStr := extractEpisodeRange(cleanedName)
		if rangeStr != "" {
			if s != 0 {
				seriesParts = fmt.Sprintf("S%02d %s", s, rangeStr)
			} else {
				seriesParts = rangeStr
			}
		} else if s != 0 && e != 0 {
			seriesParts = fmt.Sprintf("S%02dE%02d", s, e)
		} else if s != 0 {
			seriesParts = fmt.Sprintf("S%02d", s)
		} else if e != 0 {
			seriesParts = fmt.Sprintf("E%02d", e)
		}

		titleAndSeason := title
		if seriesParts != "" {
			titleAndSeason = fmt.Sprintf("%s %s", title, seriesParts)
		}
		if year != "" {
			titleAndSeason = fmt.Sprintf("%s (%s)", titleAndSeason, year)
		}

		if infoStr != "" {
			btnText = fmt.Sprintf("📺 [%s] %s [%s]", sizeStr, titleAndSeason, infoStr)
		} else {
			btnText = fmt.Sprintf("📺 [%s] %s", sizeStr, titleAndSeason)
		}
	} else {
		movieTitle := title
		if year != "" {
			movieTitle = fmt.Sprintf("%s (%s)", movieTitle, year)
		}

		if infoStr != "" {
			btnText = fmt.Sprintf("🎬 [%s] %s [%s]", sizeStr, movieTitle, infoStr)
		} else {
			btnText = fmt.Sprintf("🎬 [%s] %s", sizeStr, movieTitle)
		}
	}

	return btnText
}

// extractEpisodeRange extracts ranges like "E01-E04" or "Combined E1-E4" from series filename.
func extractEpisodeRange(name string) string {
	lower := strings.ToLower(name)
	lower = strings.ReplaceAll(lower, "_", " ")
	lower = strings.ReplaceAll(lower, ".", " ")

	// Helper to validate if start and end episode represent a valid range
	isValidRange := func(startStr, endStr string) bool {
		startVal, err1 := strconv.Atoi(startStr)
		endVal, err2 := strconv.Atoi(endStr)
		if err1 != nil || err2 != nil {
			return false
		}
		// End episode must be greater than start episode, and not unreasonably larger
		// (e.g., range of episodes in a single file is usually small, <= 50 episodes)
		return endVal > startVal && endVal <= startVal+50 && endVal < 300
	}

	// 1. Matches patterns like "combined ep 1 to 4", "combined e1 e4", "combined e1-e4", "combined ep1-ep4"
	rxCombinedRange := regexp.MustCompile(`(?i)\bcombined\s+(?:episodes?|eps?|ep|e)?\s?(\d+)(?:\s?-\s?|\s?to\s?|\s+)e?(?:p)?\s?(\d+)\b`)
	if m := rxCombinedRange.FindStringSubmatch(lower); len(m) > 2 {
		if isValidRange(m[1], m[2]) {
			return fmt.Sprintf("Combined E%s-E%s", m[1], m[2])
		}
	}

	// 2. Matches patterns like "e1-e4", "e01-e04", "ep1-ep4", "episodes 1-4", "eps 1 to 4"
	rxRange := regexp.MustCompile(`(?i)\b(?:episodes?|eps?|ep|e)\s?(\d+)(?:\s?-\s?|\s?to\s?|\s+)e?(?:p)?\s?(\d+)\b`)
	if m := rxRange.FindStringSubmatch(lower); len(m) > 2 {
		if isValidRange(m[1], m[2]) {
			return fmt.Sprintf("E%s-E%s", m[1], m[2])
		}
	}

	// 3. Matches simple digits range like "1-4" or "01-04" when preceded by "episode" or similar
	rxSimple := regexp.MustCompile(`(?i)\b(?:episodes?|eps?|ep|e)?\s*(\d+)\s*-\s*(\d+)\b`)
	if m := rxSimple.FindStringSubmatch(lower); len(m) > 2 {
		if isValidRange(m[1], m[2]) {
			return fmt.Sprintf("E%s-E%s", m[1], m[2])
		}
	}

	// 4. Fallback to just "Combined" if the word combined is in the file name
	if strings.Contains(lower, "combined") {
		return "Combined"
	}

	return ""
}

func isAlphaNum(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// SortSeries sorts files by Season (ASC), Episode (ASC), and Quality (DESC).
func (files Files) SortSeries() {
	slices.SortFunc(files, func(a, b File) int {
		s1, e1 := ExtractSeriesMetadata(a.FileName)
		s2, e2 := ExtractSeriesMetadata(b.FileName)
		
		if s1 != s2 {
			return s1 - s2
		}
		if e1 != e2 {
			return e1 - e2
		}
		
		q1 := QualityLevel(a.FileName)
		q2 := QualityLevel(b.FileName)
		return q2 - q1 // Quality DESC
	})
}

// SortMovies sorts files by Quality (DESC), Size (DESC), and Name (ASC).
func (files Files) SortMovies() {
	slices.SortFunc(files, func(a, b File) int {
		q1 := QualityLevel(a.FileName)
		q2 := QualityLevel(b.FileName)
		if q1 != q2 {
			return q2 - q1 // Quality DESC
		}
		
		if a.FileSize != b.FileSize {
			if b.FileSize > a.FileSize {
				return 1
			}
			return -1
		}
		
		return 0
	})
}

// FilterBySeason returns only files belonging to a specific season.
func (files Files) FilterBySeason(s int) Files {
	result := make(Files, 0, len(files))
	for _, f := range files {
		fs, _ := ExtractSeriesMetadata(f.FileName)
		if fs == s {
			result = append(result, f)
		}
	}
	return result
}

// FilterByLanguage returns only files containing the specified language tag.
func (files Files) FilterByLanguage(lang string) Files {
	result := make(Files, 0, len(files))
	lang = strings.ToLower(lang)
	
	patterns := map[string][]string{
		"hindi":     {"hindi", "hin"},
		"english":   {"english", "eng"},
		"tamil":     {"tamil", "tam"},
		"telugu":    {"telugu", "tel"},
		"malayalam": {"malayalam", "mal"},
		"kannada":   {"kannada", "kan"},
		"multi":     {"multi", "dual", "mux"},
	}

	searchTags, ok := patterns[lang]
	if !ok {
		// If not in our patterns, just try literal match
		searchTags = []string{lang}
	}

	for _, f := range files {
		lower := strings.ToLower(f.FileName)
		found := false
		for _, tag := range searchTags {
			if strings.Contains(lower, tag) {
				found = true
				break
			}
		}
		if found {
			result = append(result, f)
		}
	}
	return result
}

// ProcessSeasons returns a keyboard with season buttons for series.
func (files Files) ProcessSeasons(uniqueId string) [][]gotgbot.InlineKeyboardButton {
	groups := GroupBySeason(files)
	keyboard := make([][]gotgbot.InlineKeyboardButton, 0, (len(groups)/2)+1)
	
	// Sort seasons for consistent UI
	seasons := make([]int, 0, len(groups))
	for s := range groups {
		seasons = append(seasons, s)
	}
	slices.Sort(seasons)

	var currentRow []gotgbot.InlineKeyboardButton
	for _, s := range seasons {
		if s == 0 {
			continue // Skip 'Other' button for cleaner UI
		}
		text := fmt.Sprintf("💠 S%d", s)
		currentRow = append(currentRow, gotgbot.InlineKeyboardButton{
			Text:         text,
			CallbackData: fmt.Sprintf("sn|%s_%d", uniqueId, s),
			Style:        "primary",
		})
		if len(currentRow) == 2 {
			keyboard = append(keyboard, currentRow)
			currentRow = nil
		}
	}
	if len(currentRow) > 0 {
		keyboard = append(keyboard, currentRow)
	}

	return keyboard
}

// SelectMenu returns a keyboard with to select files from.
func (files Files) SelectMenu(uniqueId string, pageIndex int) [][]gotgbot.InlineKeyboardButton {
	keyboard := make([][]gotgbot.InlineKeyboardButton, 0, len(files))

	for _, f := range files {
		btn := gotgbot.InlineKeyboardButton{
			Text:         fmt.Sprintf("%s[%s] %s", tick(f.IsSelected), functions.FileSizeToString(f.FileSize), CleanFileNameForDisplay(f.FileName)),
			CallbackData: fmt.Sprintf("sel|%s_%d_%s", uniqueId, pageIndex, f.UniqueId),
		}
		if f.IsSelected {
			btn.Style = "primary"
		}
		keyboard = append(keyboard, []gotgbot.InlineKeyboardButton{btn})
	}

	return keyboard
}

// tick returns a tick symbol if val is true.
func tick(val bool) string {
	if val {
		return "✅ "
	}

	return ""
}

// CleanFileNameForDisplay replaces all dots in filename with spaces and removes common extension.
func CleanFileNameForDisplay(name string) string {
	name = functions.CleanPromoFromName(name)
	if idx := strings.LastIndex(name, "."); idx != -1 {
		ext := name[idx:]
		if len(ext) <= 5 {
			name = name[:idx]
		}
	}
	name = strings.ReplaceAll(name, ".", " ")
	return strings.Join(strings.Fields(name), " ")
}
