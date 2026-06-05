package autofilter

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	posterClient = &http.Client{Timeout: 5 * time.Second}
	posterCache  = make(map[string]string)
	posterMu     sync.RWMutex
)

// CleanQueryForPoster removes season, episode, and quality indicators from the search query.
func CleanQueryForPoster(query string) string {
	lower := strings.ToLower(query)
	// Remove season/episode indicators
	lower = seasonEpisodeRegex.ReplaceAllString(lower, " ")
	lower = seasonOnlyRegex.ReplaceAllString(lower, " ")
	// Remove quality tags
	lower = qualityRegex.ReplaceAllString(lower, " ")
	// Replace non-alphanumeric with spaces, except colon/apostrophe
	r := regexp.MustCompile(`[\.\-_\(\)\[\]\{\}\+\*]`)
	lower = r.ReplaceAllString(lower, " ")
	// Remove extra spaces
	fields := strings.Fields(lower)
	return strings.Join(fields, " ")
}

// GetPosterUrl fetches a poster image URL for the query from TMDB or OMDB APIs.
func GetPosterUrl(query string) string {
	return GetPosterUrlWithType(query, false)
}

// GetPosterUrlWithType fetches a poster image URL prioritizing media type (movie vs tv series).
// It validates that the API result title matches the query to avoid showing wrong posters.
func GetPosterUrlWithType(query string, isSeries bool) string {
	cleanQuery := CleanQueryForPoster(query)
	if cleanQuery == "" {
		return ""
	}

	cacheKey := fmt.Sprintf("%s_%t", cleanQuery, isSeries)

	posterMu.RLock()
	if cached, ok := posterCache[cacheKey]; ok {
		posterMu.RUnlock()
		return cached
	}
	posterMu.RUnlock()

	var resultUrl string

	// 1. Try TMDB
	tmdbKey := os.Getenv("TMDB_API_KEY")
	if tmdbKey != "" {
		queryYear := extractYear(cleanQuery)
		titleWithoutYear := cleanQuery
		if queryYear != "" {
			titleWithoutYear = strings.ReplaceAll(cleanQuery, queryYear, "")
			titleWithoutYear = strings.TrimSpace(titleWithoutYear)
		}
		if titleWithoutYear == "" {
			titleWithoutYear = cleanQuery
		}

		var reqUrl string
		if isSeries {
			if queryYear != "" {
				reqUrl = fmt.Sprintf("https://api.themoviedb.org/3/search/tv?query=%s&first_air_date_year=%s", url.QueryEscape(titleWithoutYear), queryYear)
			} else {
				reqUrl = fmt.Sprintf("https://api.themoviedb.org/3/search/tv?query=%s", url.QueryEscape(cleanQuery))
			}
		} else {
			if queryYear != "" {
				reqUrl = fmt.Sprintf("https://api.themoviedb.org/3/search/movie?query=%s&primary_release_year=%s", url.QueryEscape(titleWithoutYear), queryYear)
			} else {
				reqUrl = fmt.Sprintf("https://api.themoviedb.org/3/search/movie?query=%s", url.QueryEscape(cleanQuery))
			}
		}

		req, err := http.NewRequest("GET", reqUrl, nil)
		if err == nil {
			req.Header.Set("Authorization", "Bearer "+tmdbKey)
			req.Header.Set("accept", "application/json")
			resp, err := posterClient.Do(req)
			if err == nil {
				defer resp.Body.Close()
				if resp.StatusCode == 200 {
					var result struct {
						Results []struct {
							PosterPath    string `json:"poster_path"`
							Title         string `json:"title"`
							OriginalTitle string `json:"original_title"`
							Name          string `json:"name"`
							OriginalName  string `json:"original_name"`
							ReleaseDate   string `json:"release_date"`
							FirstAirDate  string `json:"first_air_date"`
						} `json:"results"`
					}
					if err := json.NewDecoder(resp.Body).Decode(&result); err == nil {
						for _, r := range result.Results {
							title := r.Title
							if title == "" {
								title = r.Name
							}
							originalTitle := r.OriginalTitle
							if originalTitle == "" {
								originalTitle = r.OriginalName
							}
							if !isASCII(title) && isASCII(originalTitle) && originalTitle != "" {
								title = originalTitle
							}
							apiDate := r.ReleaseDate
							if apiDate == "" {
								apiDate = r.FirstAirDate
							}
							if r.PosterPath != "" && matchYear(apiDate, cleanQuery) && (titleMatchesQuery(title, cleanQuery) || (originalTitle != "" && titleMatchesQuery(originalTitle, cleanQuery))) {
								resultUrl = "https://image.tmdb.org/t/p/w500" + r.PosterPath
								break
							}
						}
					}
				}
			}
		}

		// Fallback to search/multi if specific search failed or returned no results
		if resultUrl == "" {
			multiUrl := fmt.Sprintf("https://api.themoviedb.org/3/search/multi?query=%s", url.QueryEscape(cleanQuery))
			req, err := http.NewRequest("GET", multiUrl, nil)
			if err == nil {
				req.Header.Set("Authorization", "Bearer "+tmdbKey)
				req.Header.Set("accept", "application/json")
				resp, err := posterClient.Do(req)
				if err == nil {
					defer resp.Body.Close()
					if resp.StatusCode == 200 {
						var result struct {
							Results []struct {
								PosterPath    string `json:"poster_path"`
								MediaType     string `json:"media_type"`
								Title         string `json:"title"`
								OriginalTitle string `json:"original_title"`
								Name          string `json:"name"`
								OriginalName  string `json:"original_name"`
								ReleaseDate   string `json:"release_date"`
								FirstAirDate  string `json:"first_air_date"`
							} `json:"results"`
						}
						if err := json.NewDecoder(resp.Body).Decode(&result); err == nil {
							// Pass 1: Try to find exact title match
							for _, r := range result.Results {
								title := r.Title
								if title == "" {
									title = r.Name
								}
								originalTitle := r.OriginalTitle
								if originalTitle == "" {
									originalTitle = r.OriginalName
								}
								if !isASCII(title) && isASCII(originalTitle) && originalTitle != "" {
									title = originalTitle
								}
								apiDate := r.ReleaseDate
								if apiDate == "" {
									apiDate = r.FirstAirDate
								}
								if r.PosterPath == "" || !matchYear(apiDate, cleanQuery) {
									continue
								}
								if strings.EqualFold(strings.TrimSpace(title), strings.TrimSpace(cleanQuery)) || (originalTitle != "" && strings.EqualFold(strings.TrimSpace(originalTitle), strings.TrimSpace(cleanQuery))) {
									if isSeries && r.MediaType == "tv" {
										resultUrl = "https://image.tmdb.org/t/p/w500" + r.PosterPath
										break
									} else if !isSeries && r.MediaType == "movie" {
										resultUrl = "https://image.tmdb.org/t/p/w500" + r.PosterPath
										break
									}
								}
							}

							// Pass 2: Try to find matching media_type with title validation (partial match)
							if resultUrl == "" {
								for _, r := range result.Results {
									title := r.Title
									if title == "" {
										title = r.Name
									}
									originalTitle := r.OriginalTitle
									if originalTitle == "" {
										originalTitle = r.OriginalName
									}
									if !isASCII(title) && isASCII(originalTitle) && originalTitle != "" {
										title = originalTitle
									}
									apiDate := r.ReleaseDate
									if apiDate == "" {
										apiDate = r.FirstAirDate
									}
									if r.PosterPath == "" || !matchYear(apiDate, cleanQuery) {
										continue
									}
									if !titleMatchesQuery(title, cleanQuery) && (originalTitle == "" || !titleMatchesQuery(originalTitle, cleanQuery)) {
										continue
									}
									if isSeries && r.MediaType == "tv" {
										resultUrl = "https://image.tmdb.org/t/p/w500" + r.PosterPath
										break
									} else if !isSeries && r.MediaType == "movie" {
										resultUrl = "https://image.tmdb.org/t/p/w500" + r.PosterPath
										break
									}
								}
							}
							// Fallback: accept any media type if title matches
							if resultUrl == "" {
								for _, r := range result.Results {
									title := r.Title
									if title == "" {
										title = r.Name
									}
									originalTitle := r.OriginalTitle
									if originalTitle == "" {
										originalTitle = r.OriginalName
									}
									if !isASCII(title) && isASCII(originalTitle) && originalTitle != "" {
										title = originalTitle
									}
									apiDate := r.ReleaseDate
									if apiDate == "" {
										apiDate = r.FirstAirDate
									}
									if r.PosterPath != "" && matchYear(apiDate, cleanQuery) && (titleMatchesQuery(title, cleanQuery) || (originalTitle != "" && titleMatchesQuery(originalTitle, cleanQuery))) {
										resultUrl = "https://image.tmdb.org/t/p/w500" + r.PosterPath
										break
									}
								}
							}
						}
					}
				}
			}
		}
	}

	// 2. Try OMDB
	if resultUrl == "" {
		omdbKey := os.Getenv("OMDB_API_KEY")
		if omdbKey != "" {
			queryYear := extractYear(cleanQuery)
			titleWithoutYear := cleanQuery
			if queryYear != "" {
				titleWithoutYear = strings.ReplaceAll(cleanQuery, queryYear, "")
				titleWithoutYear = strings.TrimSpace(titleWithoutYear)
			}
			if titleWithoutYear == "" {
				titleWithoutYear = cleanQuery
			}

			var typeParam string
			if isSeries {
				typeParam = "&type=series"
			} else {
				typeParam = "&type=movie"
			}

			// Attempt 1: Direct Title Lookup
			var directUrl string
			if queryYear != "" {
				directUrl = fmt.Sprintf("http://www.omdbapi.com/?apikey=%s&t=%s&y=%s%s", omdbKey, url.QueryEscape(titleWithoutYear), queryYear, typeParam)
			} else {
				directUrl = fmt.Sprintf("http://www.omdbapi.com/?apikey=%s&t=%s%s", omdbKey, url.QueryEscape(cleanQuery), typeParam)
			}

			respDirect, err := posterClient.Get(directUrl)
			if err == nil {
				defer respDirect.Body.Close()
				if respDirect.StatusCode == 200 {
					var directResult struct {
						Poster   string `json:"Poster"`
						Title    string `json:"Title"`
						Year     string `json:"Year"`
						Response string `json:"Response"`
					}
					if err := json.NewDecoder(respDirect.Body).Decode(&directResult); err == nil {
						if directResult.Response == "True" && directResult.Poster != "" && directResult.Poster != "N/A" && matchYear(directResult.Year, cleanQuery) && titleMatchesQuery(directResult.Title, cleanQuery) {
							resultUrl = directResult.Poster
						}
					}
				}
			}

			// Attempt 2: Search Lookup Fallback
			if resultUrl == "" {
				reqUrl := fmt.Sprintf("http://www.omdbapi.com/?apikey=%s&s=%s%s", omdbKey, url.QueryEscape(cleanQuery), typeParam)
				resp, err := posterClient.Get(reqUrl)
				if err == nil {
					defer resp.Body.Close()
					if resp.StatusCode == 200 {
						var result struct {
							Search []struct {
								Poster string `json:"Poster"`
								Title  string `json:"Title"`
								Year   string `json:"Year"`
							} `json:"Search"`
							Response string `json:"Response"`
						}
						if err := json.NewDecoder(resp.Body).Decode(&result); err == nil {
							if result.Response == "True" {
								for _, r := range result.Search {
									if r.Poster != "" && r.Poster != "N/A" && matchYear(r.Year, cleanQuery) && titleMatchesQuery(r.Title, cleanQuery) {
										resultUrl = r.Poster
										break
									}
								}
							}
						}
					}
				}
			}
		}
	}

	posterMu.Lock()
	posterCache[cacheKey] = resultUrl
	posterMu.Unlock()

	return resultUrl
}

// GetSeasonPosterUrl fetches a season-specific poster for a TV series.
func GetSeasonPosterUrl(query string, season int) string {
	cleanQuery := CleanQueryForPoster(query)
	if cleanQuery == "" {
		return ""
	}

	cacheKey := fmt.Sprintf("%s_s%d", cleanQuery, season)

	posterMu.RLock()
	if cached, ok := posterCache[cacheKey]; ok {
		posterMu.RUnlock()
		return cached
	}
	posterMu.RUnlock()

	var resultUrl string
	tmdbKey := os.Getenv("TMDB_API_KEY")
	if tmdbKey != "" {
		// 1. Search for the TV show to get its ID
		searchUrl := fmt.Sprintf("https://api.themoviedb.org/3/search/tv?query=%s", url.QueryEscape(cleanQuery))
		req, err := http.NewRequest("GET", searchUrl, nil)
		if err == nil {
			req.Header.Set("Authorization", "Bearer "+tmdbKey)
			req.Header.Set("accept", "application/json")
			resp, err := posterClient.Do(req)
			if err == nil {
				defer resp.Body.Close()
				if resp.StatusCode == 200 {
					var searchResult struct {
						Results []struct {
							ID           int    `json:"id"`
							Name         string `json:"name"`
							OriginalName string `json:"original_name"`
						} `json:"results"`
					}
					if err := json.NewDecoder(resp.Body).Decode(&searchResult); err == nil && len(searchResult.Results) > 0 {
						// Find the best matching show
						var showID int
						for _, r := range searchResult.Results {
							if titleMatchesQuery(r.Name, cleanQuery) || titleMatchesQuery(r.OriginalName, cleanQuery) {
								showID = r.ID
								break
							}
						}
						if showID == 0 {
							showID = searchResult.Results[0].ID
						}

						// 2. Query the TV details to get seasons
						if showID > 0 {
							detailsUrl := fmt.Sprintf("https://api.themoviedb.org/3/tv/%d", showID)
							reqDetails, err := http.NewRequest("GET", detailsUrl, nil)
							if err == nil {
								reqDetails.Header.Set("Authorization", "Bearer "+tmdbKey)
								reqDetails.Header.Set("accept", "application/json")
								respDetails, err := posterClient.Do(reqDetails)
								if err == nil {
									defer respDetails.Body.Close()
									if respDetails.StatusCode == 200 {
										var detailsResult struct {
											Seasons []struct {
												SeasonNumber int    `json:"season_number"`
												PosterPath   string `json:"poster_path"`
											} `json:"seasons"`
										}
										if err := json.NewDecoder(respDetails.Body).Decode(&detailsResult); err == nil {
											for _, s := range detailsResult.Seasons {
												if s.SeasonNumber == season {
													if s.PosterPath != "" {
														resultUrl = "https://image.tmdb.org/t/p/w500" + s.PosterPath
													}
													break
												}
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}

	// Fallback to the main show poster if season-specific poster wasn't found
	if resultUrl == "" {
		resultUrl = GetPosterUrlWithType(query, true)
	}

	posterMu.Lock()
	posterCache[cacheKey] = resultUrl
	posterMu.Unlock()

	return resultUrl
}


// titleMatchesQuery checks whether the API result title is a valid match for the user's search query.
// All words from the query (except 4-digit years) must appear in the title.
func titleMatchesQuery(title, query string) bool {
	if title == "" {
		return false
	}
	titleLower := strings.ToLower(title)
	queryLower := strings.ToLower(query)

	rawQueryWords := strings.Fields(queryLower)
	rawTitleWords := strings.Fields(titleLower)

	yearRegex := regexp.MustCompile(`^(19|20)\d{2}$`)

	// Filter out years from query words, unless the query contains ONLY years
	var queryWords []string
	for _, w := range rawQueryWords {
		if !yearRegex.MatchString(w) {
			queryWords = append(queryWords, w)
		}
	}
	if len(queryWords) == 0 {
		queryWords = rawQueryWords
	}

	// Filter out years from title words, unless the title contains ONLY years
	var titleWords []string
	for _, w := range rawTitleWords {
		if !yearRegex.MatchString(w) {
			titleWords = append(titleWords, w)
		}
	}
	if len(titleWords) == 0 {
		titleWords = rawTitleWords
	}

	// All query words must be present in the title
	for _, qw := range queryWords {
		found := false
		for _, tw := range titleWords {
			if tw == qw {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Title must not have significantly more words than the query (prevents partial match to longer titles)
	// Allow up to 3 extra words (for articles, subtitles, etc.)
	if len(titleWords) > len(queryWords)+3 {
		return false
	}

	return true
}

// GetSearchSuggestions queries TMDB and OMDB in parallel to find up to 5 matching movie/series titles (clean titles).
func GetSearchSuggestions(query string) []string {
	cleanQuery := CleanQueryForPoster(query)
	if cleanQuery == "" {
		return nil
	}

	resChan := make(chan []string, 2)

	// We use a short timeout client for suggestions to keep the bot responsive
	fastClient := &http.Client{Timeout: 800 * time.Millisecond}

	// 1. Try TMDB in parallel
	go func() {
		var tmdbSug []string
		tmdbKey := os.Getenv("TMDB_API_KEY")
		if tmdbKey != "" {
			reqUrl := fmt.Sprintf("https://api.themoviedb.org/3/search/multi?query=%s", url.QueryEscape(cleanQuery))
			req, err := http.NewRequest("GET", reqUrl, nil)
			if err == nil {
				req.Header.Set("Authorization", "Bearer "+tmdbKey)
				req.Header.Set("accept", "application/json")
				resp, err := fastClient.Do(req)
				if err == nil {
					defer resp.Body.Close()
					if resp.StatusCode == 200 {
						var result struct {
							Results []struct {
								Title         string `json:"title"`
								OriginalTitle string `json:"original_title"`
								Name          string `json:"name"`
								OriginalName  string `json:"original_name"`
								ReleaseDate   string `json:"release_date"`
								FirstAirDate  string `json:"first_air_date"`
							} `json:"results"`
						}
						if err := json.NewDecoder(resp.Body).Decode(&result); err == nil {
							for _, r := range result.Results {
								if len(tmdbSug) >= 5 {
									break
								}
								title := r.Title
								if title == "" {
									title = r.Name
								}
								originalTitle := r.OriginalTitle
								if originalTitle == "" {
									originalTitle = r.OriginalName
								}
								if !isASCII(title) && isASCII(originalTitle) && originalTitle != "" {
									title = originalTitle
								}
								if title == "" {
									continue
								}
								date := r.ReleaseDate
								if date == "" {
									date = r.FirstAirDate
								}
								if len(date) >= 4 {
									title = fmt.Sprintf("%s (%s)", title, date[:4])
								}
								tmdbSug = append(tmdbSug, title)
							}
						}
					}
				}
			}
		}
		resChan <- tmdbSug
	}()

	// 2. Try OMDB in parallel
	go func() {
		var omdbSug []string
		omdbKey := os.Getenv("OMDB_API_KEY")
		if omdbKey != "" {
			reqUrl := fmt.Sprintf("http://www.omdbapi.com/?apikey=%s&s=%s", omdbKey, url.QueryEscape(cleanQuery))
			resp, err := fastClient.Get(reqUrl)
			if err == nil {
				defer resp.Body.Close()
				if resp.StatusCode == 200 {
					var result struct {
						Search []struct {
							Title string `json:"Title"`
							Year  string `json:"Year"`
						} `json:"Search"`
						Response string `json:"Response"`
					}
					if err := json.NewDecoder(resp.Body).Decode(&result); err == nil {
						if result.Response == "True" {
							for _, r := range result.Search {
								if len(omdbSug) >= 5 {
									break
								}
								title := r.Title
								if title == "" {
									continue
								}
								if r.Year != "" {
									year := r.Year
									if len(year) > 4 {
										year = year[:4]
									}
									title = fmt.Sprintf("%s (%s)", title, year)
								}
								omdbSug = append(omdbSug, title)
							}
						}
					}
				}
			}
		}
		resChan <- omdbSug
	}()

	var allSuggestions []string
	seen := make(map[string]bool)

	addSuggestion := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		lower := strings.ToLower(s)
		if !seen[lower] {
			seen[lower] = true
			allSuggestions = append(allSuggestions, s)
		}
	}

	// Read from channel 2 times
	for i := 0; i < 2; i++ {
		select {
		case sugs := <-resChan:
			for _, s := range sugs {
				addSuggestion(s)
			}
		case <-time.After(800 * time.Millisecond):
			break
		}
	}

	if len(allSuggestions) > 5 {
		allSuggestions = allSuggestions[:5]
	}

	return allSuggestions
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] > 127 {
			return false
		}
	}
	return true
}

func matchYear(apiDateOrYear, query string) bool {
	queryYear := extractYear(query)
	if queryYear == "" {
		return true
	}
	apiYear := extractYear(apiDateOrYear)
	if apiYear == "" {
		return true
	}
	return apiYear == queryYear
}

func extractYear(s string) string {
	r := regexp.MustCompile(`\b(19|20)\d{2}\b`)
	return r.FindString(s)
}
