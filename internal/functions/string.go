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
	if (len(input) - index) <= 4 { // if last index of . is within 4 character range of end of string then cut around it
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
