package autofilter

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"autofilterbot/internal/functions"
)

var (
	seasonEpisodeRegex = regexp.MustCompile(`(?i)\bs(\d+)\s?e(\d+)\b|\bseason\s?(\d+)\s?episode\s?(\d+)\b|\b(\d+)x(\d+)\b`)
	qualityRegex       = regexp.MustCompile(`(?i)(\d{3,4}p|bluray|web-dl|webrip|hdtv|camrip|brrip|h264|h265|x264|x265|dvdrip|hdrip|tc|ts|scr|hevc|hq|dd5\.1|\b(?:color|bw|b&w|black\s+and\s+white|imax|remastered|extended|unrated|directors?\s+cut|dc|special\s+edition|se|proper|repack)\b)`)
	seasonOnlyRegex    = regexp.MustCompile(`(?i)\bseason\s?(\d+)\b|\bs(\d+)\b`)
	episodeOnlyRegex   = regexp.MustCompile(`(?i)\bepisode\s?(\d+)\b|\bep[._-]?\s?(\d+)\b|\be(\d{1,4})\b`)
	yearRegex          = regexp.MustCompile(`\b(19|20)\d{2}\b`)
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
	case strings.Contains(lower, "2160p") || strings.Contains(lower, "4k"):
		return 100
	case strings.Contains(lower, "1080p"):
		return 80
	case strings.Contains(lower, "bluray"):
		return 75
	case strings.Contains(lower, "720p"):
		return 60
	case strings.Contains(lower, "hdtv") || strings.Contains(lower, "web"):
		return 50
	case strings.Contains(lower, "480p"):
		return 40
	case strings.Contains(lower, "hdrip"):
		return 35
	case strings.Contains(lower, "webrip"):
		return 30
	case strings.Contains(lower, "dvdrip"):
		return 25
	case strings.Contains(lower, "hdtc") || strings.Contains(lower, "tc") || strings.Contains(lower, "telecine"):
		return 15
	case strings.Contains(lower, "camrip") || strings.Contains(lower, "cam") || strings.Contains(lower, "hdcam") || strings.Contains(lower, "ts"):
		return 10
	case strings.Contains(lower, "predvd") || strings.Contains(lower, "p-dvd"):
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
	garbage := []string{"sample", "trailer", ".srt", ".txt", "nfo", "idx", "sub", "camrip", "predvd", "hdcam", "telecine", "hdtc", "p-dvd"}
	return slices.ContainsFunc(garbage, func(g string) bool {
		return strings.Contains(lower, g)
	})
}
var languageRegexes = map[string]*regexp.Regexp{
	"Hindi":     regexp.MustCompile(`(?i)(?:[^a-zA-Z0-9]|^)(hindi|hin)(?:[^a-zA-Z0-9]|$)`),
	"English":   regexp.MustCompile(`(?i)(?:[^a-zA-Z0-9]|^)(english|eng)(?:[^a-zA-Z0-9]|$)`),
	"Tamil":     regexp.MustCompile(`(?i)(?:[^a-zA-Z0-9]|^)(tamil|tam)(?:[^a-zA-Z0-9]|$)`),
	"Telugu":    regexp.MustCompile(`(?i)(?:[^a-zA-Z0-9]|^)(telugu|tel)(?:[^a-zA-Z0-9]|$)`),
	"Malayalam": regexp.MustCompile(`(?i)(?:[^a-zA-Z0-9]|^)(malayalam|mal)(?:[^a-zA-Z0-9]|$)`),
	"Kannada":   regexp.MustCompile(`(?i)(?:[^a-zA-Z0-9]|^)(kannada|kan)(?:[^a-zA-Z0-9]|$)`),
	"Multi":     regexp.MustCompile(`(?i)(?:[^a-zA-Z0-9]|^)(multi|dual|mux)(?:[^a-zA-Z0-9]|$)`),
}

// DetectLanguages extracts available languages from the file list.
func DetectLanguages(files []File) []string {
	langs := make(map[string]bool)

	for _, f := range files {
		for name, regex := range languageRegexes {
			if regex.MatchString(f.FileName) {
				langs[name] = true
			}
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
	title = strings.TrimRight(title, "([]{}-_ ")
	title = strings.TrimSpace(title)
	title = strings.Join(strings.Fields(title), " ")

	// Title case
	if title != "" {
		title = strings.Title(strings.ToLower(title))
	}

	return title
}
