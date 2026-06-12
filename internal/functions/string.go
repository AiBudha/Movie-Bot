package functions

import (
	"crypto/rand"
	"math/big"
	"regexp"
	"strings"
)

var nonAlphaNumericRegex = regexp.MustCompile(`[^\w\s]+`)

var promoPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(?:\[\s*@\w+\s*\]|\(\s*@\w+\s*\)|@\w+)`),
	regexp.MustCompile(`(?i)t\.me/\w+`),
	regexp.MustCompile(`^\s*\[\s*[A-Za-z]{2,3}\s*\][\s._-]*`),
	regexp.MustCompile(`^\s*@\w+(?:\s+[Xx])?\b[\s._-]*`),
	regexp.MustCompile(`^\s*(?i)(?:\[\s*(?:mm|cc|mlm|dramaost|kc|tg|tc|km|kck|mmc|ms|pa|psa|yts|mkv|rxtv|ds)\s*\]|\(\s*(?:mm|cc|mlm|dramaost|kc|tg|tc|km|kck|mmc|ms|pa|psa|yts|mkv|rxtv|ds)\s*\)|(?:mm|cc|mlm|dramaost|kc|tg|tc|km|kck|mmc|ms|pa|psa|yts|mkv|rxtv|ds))(?:_|\b)[\s._-]*`),
	regexp.MustCompile(`(?i)\bDramaOST\b`),
}

// CleanPromoFromName removes username promotions and links from filename.
func CleanPromoFromName(name string) string {
	for _, pattern := range promoPatterns {
		name = pattern.ReplaceAllString(name, " ")
	}
	return strings.TrimSpace(strings.Join(strings.Fields(name), " "))
}

// RemoveSymbols returns a copy of the string will all non alpha-numeric characters removed.
func RemoveSymbols(input string) string {
	input = strings.ReplaceAll(input, "_", " ")
	// removes all symbols using regex and then splits into fields and rejoins to remove unnecessary whitespaces
	return strings.Join(strings.Fields(nonAlphaNumericRegex.ReplaceAllString(input, " ")), " ")
}

// RemoveExtension removes the extension from a file name if any.
func RemoveExtension(input string) string {
	if input == "" {
		return ""
	}

	index := strings.LastIndex(input, ".")
	if (len(input) - index) <= 5 { // if last index of . is within 5 character range of end of string then cut around it
		input = input[:index]
	}

	return input
}

const (
	charset    = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	lenCharSet = int64(len(charset))
)

// RandString creates a randomly generated string of given length.
func RandString(length int) string {
	b := make([]byte, length)

	for i := range b {
		randIndex, _ := rand.Int(rand.Reader, big.NewInt(lenCharSet))
		b[i] = charset[randIndex.Int64()]
	}

	return string(b)
}

// Levenshtein calculates the Levenshtein distance between two strings.
func Levenshtein(s, t string) int {
	s = strings.ToLower(s)
	t = strings.ToLower(t)
	d := make([][]int, len(s)+1)
	for i := range d {
		d[i] = make([]int, len(t)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for i := 1; i <= len(s); i++ {
		for j := 1; j <= len(t); j++ {
			if s[i-1] == t[j-1] {
				d[i][j] = d[i-1][j-1]
			} else {
				min := d[i-1][j]
				if d[i][j-1] < min {
					min = d[i][j-1]
				}
				if d[i-1][j-1] < min {
					min = d[i-1][j-1]
				}
				d[i][j] = min + 1
			}
		}
	}
	return d[len(s)][len(t)]
}

