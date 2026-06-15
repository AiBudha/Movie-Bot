package autofilter

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"autofilterbot/internal/functions"
)

var (
	seasonEpisodeRegex = regexp.MustCompile(`(?i)\bs[o0]?(\d+)[._\s-]*(?:e|ep|episode|ep\.)[o0]?\s?(\d+)\b|\bseason\s?(\d+)\s?episode\s?(\d+)\b|\b(\d+)x(\d+)\b`)
	qualityRegex       = regexp.MustCompile(`(?i)\b(\d{3,4}p|bluray|web-dl|webrip|hdtv|camrip|brrip|h264|h265|x264|x265|dvdrip|hdrip|tc|ts|scr|hevc|hq|dd5\.1|color|bw|b&w|black\s+and\s+white|imax|remastered|extended|unrated|directors?\s+cut|dc|special\s+edition|se|proper|repack)\b`)
	seasonOnlyRegex    = regexp.MustCompile(`(?i)\bseason\s?(\d+)\b|\bs(\d+)\b`)
	episodeOnlyRegex   = regexp.MustCompile(`(?i)\bepisode\s?(\d+)\b|\bep[._-]?\s?(\d+)\b|\be(\d{1,4})\b`)
	yearRegex          = regexp.MustCompile(`\b(19|20)\d{2}\b`)

	quality2160pRegex  = regexp.MustCompile(`(?i)\b(2160p|4k)\b`)
	quality1080pRegex  = regexp.MustCompile(`(?i)\b1080p\b`)
	quality720pRegex   = regexp.MustCompile(`(?i)\b720p\b`)
	quality480pRegex   = regexp.MustCompile(`(?i)\b480p\b`)
	qualityBlurayRegex = regexp.MustCompile(`(?i)\bbluray\b`)
	qualityWebRegex    = regexp.MustCompile(`(?i)\b(hdtv|web|web-dl|webrip)\b`)
	qualityHdripRegex  = regexp.MustCompile(`(?i)\bhdrip\b`)
	qualityDvdripRegex = regexp.MustCompile(`(?i)\bdvdrip\b`)

	qualityTcRegex     = regexp.MustCompile(`(?i)\b(hdtc|tc|telecine)\b`)
	qualityCamRegex    = regexp.MustCompile(`(?i)\b(camrip|cam|hdcam|ts|telesync|screener|dvdscr|scr|hqcam|hq-cam|hc)\b`)
	qualityPredvdRegex = regexp.MustCompile(`(?i)\b(predvd|p-dvd|pre-dvd)\b`)

	garbageRegex       = regexp.MustCompile(`(?i)\b(sample|trailer|camrip|predvd|hdcam|telecine|hdtc|p-dvd|telesync|screener|dvdscr|scr|pre-dvd|hq-cam|hqcam|hc|tc|ts|cam)\b`)
)

type MovieMetadata struct {
	Quality    string
	Resolution string
}

type SeriesMetadata struct {
	Season  int
	Episode int
}

// DetectType returns "series" if at least 40% of the returned files follow a series pattern, else "movie".
func DetectType(files []File) string {
	if len(files) == 0 {
		return "movie"
	}
	seriesCount := 0
	for _, f := range files {
		if IsSeriesFile(f.FileName) {
			seriesCount++
		}
	}
	if float64(seriesCount)/float64(len(files)) >= 0.4 {
		return "series"
	}
	return "movie"
}

// IsSeriesFile returns true if the filename matches a season/episode pattern.
func IsSeriesFile(name string) bool {
	lower := strings.ToLower(name)
	lower = strings.ReplaceAll(lower, "_", " ")
	lower = strings.ReplaceAll(lower, ".", " ")
	return seasonEpisodeRegex.MatchString(lower) || seasonOnlyRegex.MatchString(lower) || episodeOnlyRegex.MatchString(lower)
}

// GroupBySeason groups files by their season number.
func GroupBySeason(files []File) map[int][]File {
	groups := make(map[int][]File)
	for _, f := range files {
		s, _ := ExtractSeriesMetadata(f.FileName)
		groups[s] = append(groups[s], f)
	}
	return groups
}

// ExtractSeriesMetadata parses season and episode from filename.
func ExtractSeriesMetadata(name string) (int, int) {
	lower := strings.ToLower(name)
	lower = strings.ReplaceAll(lower, "_", " ")
	lower = strings.ReplaceAll(lower, ".", " ")
	
	var s, e int
	matches := seasonEpisodeRegex.FindStringSubmatch(lower)
	
	if len(matches) > 0 {
		// s01e01 pattern (matches[1] and matches[2])
		if len(matches) > 2 && matches[1] != "" && matches[2] != "" {
			s, _ = strconv.Atoi(matches[1])
			e, _ = strconv.Atoi(matches[2])
		} else if len(matches) > 4 && matches[3] != "" && matches[4] != "" {
			// season 01 episode 01 pattern (matches[3] and matches[4])
			s, _ = strconv.Atoi(matches[3])
			e, _ = strconv.Atoi(matches[4])
		} else if len(matches) > 6 && matches[5] != "" && matches[6] != "" {
			// 01x01 pattern (matches[5] and matches[6])
			s, _ = strconv.Atoi(matches[5])
			e, _ = strconv.Atoi(matches[6])
		}
	}

	// Fallback to season only if no season was found or no episode was found
	if s == 0 {
		match := seasonOnlyRegex.FindStringSubmatch(lower)
		if len(match) > 1 && match[1] != "" {
			s, _ = strconv.Atoi(match[1])
		} else if len(match) > 2 && match[2] != "" {
			s, _ = strconv.Atoi(match[2])
		}
	}

	// Fallback to episode only if no episode was found
	if e == 0 {
		match := episodeOnlyRegex.FindStringSubmatch(lower)
		for idx := 1; idx < len(match); idx++ {
			if match[idx] != "" {
				e, _ = strconv.Atoi(match[idx])
				break
			}
		}
	}
	
	return s, e
}

// QualityLevel returns a numeric priority for quality tags.
func QualityLevel(name string) int {
	lower := strings.ToLower(name)
	switch {
	case quality2160pRegex.MatchString(lower):
		return 100
	case quality1080pRegex.MatchString(lower):
		return 80
	case qualityBlurayRegex.MatchString(lower):
		return 75
	case quality720pRegex.MatchString(lower):
		return 60
	case qualityWebRegex.MatchString(lower):
		return 50
	case quality480pRegex.MatchString(lower):
		return 40
	case qualityHdripRegex.MatchString(lower):
		return 35
	case qualityDvdripRegex.MatchString(lower):
		return 25
	case qualityTcRegex.MatchString(lower):
		return 15
	case qualityCamRegex.MatchString(lower):
		return 10
	case qualityPredvdRegex.MatchString(lower):
		return 5
	default:
		return 0
	}
}

// ExtractMovieLabel creates a clean label like "1080p BluRay"
func ExtractMovieLabel(name string) string {
	lower := strings.ToLower(name)
	
	// Find the first quality tag to determine where the movie name ends
	firstQualityIdx := -1
	
	match := qualityRegex.FindStringIndex(lower)
	if match != nil {
		firstQualityIdx = match[0]
	}

	var movieName string
	if firstQualityIdx != -1 {
		movieName = name[:firstQualityIdx]
	} else {
		movieName = name
	}

	// Clean movie name (remove dots, dashes, etc.)
	movieName = strings.ReplaceAll(movieName, ".", " ")
	movieName = strings.ReplaceAll(movieName, "-", " ")
	movieName = strings.TrimSpace(movieName)
	
	// Limit movie name length to keep buttons readable
	if len(movieName) > 35 {
		movieName = movieName[:32] + "..."
	}

	// Extract quality tags
	tags := qualityRegex.FindAllString(lower, -1)
	qualityStr := ""
	for _, t := range tags {
		qualityStr += strings.ToUpper(t) + " "
	}
	qualityStr = strings.TrimSpace(qualityStr)
	
	emoji := "🎬"
	if strings.Contains(lower, "2160p") || strings.Contains(lower, "4k") {
		emoji = "💎"
	} else if strings.Contains(lower, "1080p") {
		emoji = "✨"
	} else if strings.Contains(lower, "720p") {
		emoji = "⚡"
	}

	if movieName != "" && qualityStr != "" {
		return emoji + " " + movieName + " | " + qualityStr
	} else if movieName != "" {
		return emoji + " " + movieName
	}
	return qualityStr
}

// IsGarbageFile returns true for samples, trailers, etc.
func IsGarbageFile(name string) bool {
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".srt") || strings.HasSuffix(lower, ".txt") || strings.HasSuffix(lower, ".nfo") || strings.HasSuffix(lower, ".idx") || strings.HasSuffix(lower, ".sub") {
		return true
	}
	return garbageRegex.MatchString(lower)
}
var languageRegexes = map[string]*regexp.Regexp{
	"Hindi":     regexp.MustCompile(`(?i)(?:[^a-zA-Z0-9]|^)(hindi|hin)(?:[^a-zA-Z0-9]|$)`),
	"English":   regexp.MustCompile(`(?i)(?:[^a-zA-Z0-9]|^)(english|eng)(?:[^a-zA-Z0-9]|$)`),
	"Tamil":     regexp.MustCompile(`(?i)(?:[^a-zA-Z0-9]|^)(tamil|tam)(?:[^a-zA-Z0-9]|$)`),
	"Telugu":    regexp.MustCompile(`(?i)(?:[^a-zA-Z0-9]|^)(telugu|tel)(?:[^a-zA-Z0-9]|$)`),
	"Malayalam": regexp.MustCompile(`(?i)(?:[^a-zA-Z0-9]|^)(malayalam|mal)(?:[^a-zA-Z0-9]|$)`),
	"Kannada":   regexp.MustCompile(`(?i)(?:[^a-zA-Z0-9]|^)(kannada|kan)(?:[^a-zA-Z0-9]|$)`),
	"Bengali":   regexp.MustCompile(`(?i)(?:[^a-zA-Z0-9]|^)(bengali|ben)(?:[^a-zA-Z0-9]|$)`),
	"Marathi":   regexp.MustCompile(`(?i)(?:[^a-zA-Z0-9]|^)(marathi|mar)(?:[^a-zA-Z0-9]|$)`),
	"Bhojpuri":  regexp.MustCompile(`(?i)(?:[^a-zA-Z0-9]|^)(bhojpuri)(?:[^a-zA-Z0-9]|$)`),
	"Punjabi":   regexp.MustCompile(`(?i)(?:[^a-zA-Z0-9]|^)(punjabi|pun)(?:[^a-zA-Z0-9]|$)`),
	"Gujarati":  regexp.MustCompile(`(?i)(?:[^a-zA-Z0-9]|^)(gujarati|guj)(?:[^a-zA-Z0-9]|$)`),
}

var multiRegex = regexp.MustCompile(`(?i)(?:[^a-zA-Z0-9]|^)(multi|dual|mux|dubbed|dub)(?:[^a-zA-Z0-9]|$)`)

// DetectLanguages extracts available languages from the file list.
func DetectLanguages(files []File) []string {
	langs := make(map[string]bool)

	for _, f := range files {
		for name, regex := range languageRegexes {
			if regex.MatchString(f.FileName) {
				langs[name] = true
			}
		}
		if multiRegex.MatchString(f.FileName) {
			langs["Multi"] = true
		}
	}

	result := make([]string, 0, len(langs))
	for l := range langs {
		result = append(result, l)
	}
	slices.Sort(result)
	return result
}

// ExtractBaseTitle extracts the clean base title from a filename by removing
// quality tags, season/episode patterns, file extensions, and formatting artifacts.
func ExtractBaseTitle(name string) string {
	// Insert space between 4-digit years and adjacent letters (e.g. 2019english -> 2019 english)
	yearLetterRegex := regexp.MustCompile(`(?i)\b((?:19|20)\d{2})([a-zA-Z])`)
	name = yearLetterRegex.ReplaceAllString(name, "$1 $2")
	letterYearRegex := regexp.MustCompile(`(?i)([a-zA-Z])((?:19|20)\d{2})\b`)
	name = letterYearRegex.ReplaceAllString(name, "$1 $2")

	name = functions.CleanPromoFromName(name)
	// Remove leading brackets/parentheses and promotional prefixes completely (e.g. [Cc], [Govt], Cc)
	bracketRegex := regexp.MustCompile(`^(?i)(?:\[[^\]]+\]|\([^\)]+\))\s*`)
	promoPrefixRegex := regexp.MustCompile(`^(?i)\bcc\b[._\s-]*`)
	for {
		oldName := name
		name = bracketRegex.ReplaceAllString(name, "")
		name = promoPrefixRegex.ReplaceAllString(name, "")
		name = strings.TrimSpace(name)
		if name == oldName {
			break
		}
	}

	// Remove file extension
	if idx := strings.LastIndex(name, "."); idx != -1 {
		ext := name[idx:]
		if len(ext) <= 5 {
			name = name[:idx]
		}
	}

	lower := strings.ToLower(name)
	lower = strings.ReplaceAll(lower, "_", " ")
	lower = strings.ReplaceAll(lower, ".", " ")

	// Find the earliest index of quality or season/episode patterns
	cutIdx := len(name)

	if loc := qualityRegex.FindStringIndex(lower); loc != nil && loc[0] < cutIdx {
		cutIdx = loc[0]
	}
	if loc := seasonEpisodeRegex.FindStringIndex(lower); loc != nil && loc[0] < cutIdx {
		cutIdx = loc[0]
	}
	if loc := seasonOnlyRegex.FindStringIndex(lower); loc != nil && loc[0] < cutIdx {
		cutIdx = loc[0]
	}
	if loc := yearRegex.FindStringIndex(lower); loc != nil && loc[0] > 0 && loc[0] < cutIdx {
		cutIdx = loc[0]
	}

	title := name[:cutIdx]

	// Clean formatting
	title = strings.ReplaceAll(title, ".", " ")
	title = strings.ReplaceAll(title, "-", " ")
	title = strings.ReplaceAll(title, "_", " ")
	title = strings.TrimRight(title, "([]{}-_ #@|\\/+=:~*!&?")
	title = strings.TrimSpace(title)
	title = strings.Join(strings.Fields(title), " ")

	// Strip trailing languages/audio tags from title
	langRegex := regexp.MustCompile(`(?i)\s+\b(hindi|hin|english|eng|tamil|tam|telugu|tel|malayalam|mal|kannada|kan|multi|dual|mux|dubbed|dub|org|original|sub|subs|esub|esubs|hqc|hq|clean|hevc|x264|x265|av1|rip|web-dl|webrip|hdr|10bit|movie|full\s+movie|hd|new|extended|unrated|directors?\s+cut)\b\s*$`)
	for {
		old := title
		title = langRegex.ReplaceAllString(title, "")
		title = strings.TrimSpace(title)
		if title == old {
			break
		}
	}

	// Title case
	if title != "" {
		title = strings.Title(strings.ToLower(title))
	}

	return title
}
