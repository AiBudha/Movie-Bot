package ott

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"autofilterbot/internal/database"
	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PuerkitoBio/goquery"
)

// ReleaseItem represents a parsed OTT release.
type ReleaseItem struct {
	ID          interface{} `json:"id"`
	ItemID      string      `json:"item_id"`
	Type        string      `json:"type"` // "movie" or "tv"
	Title       string      `json:"title"`
	ReleaseDate string      `json:"release_date"`
	Overview    string      `json:"overview"`
	Providers   []string    `json:"providers"`
	Poster      string      `json:"poster"`
	Rating      float64     `json:"rating"`
	GenreIDs    []int       `json:"genre_ids"`
	GenreNames  []string    `json:"genre_names"`
	Source      string      `json:"source"`
}

const (
	tmdbBase      = "https://api.themoviedb.org/3"
	tmdbImg       = "https://image.tmdb.org/t/p/w500"
	jwGqlURL      = "https://apis.justwatch.com/graphql"
	ottreleaseURL = "https://www.ottrelease.com/streaming-now"
)

// Watch Provider ID to Friendly Name
var OTTProviders = map[int]string{
	8:   "Netflix",        9:   "Amazon Prime",   337: "Disney+",
	122: "Hotstar",        2:   "Apple TV+",       384: "Max",
	386: "Peacock",       531: "Paramount+",      283: "Crunchyroll",
	11:  "MUBI",          15:  "Hulu",            103: "MX Player",
	220: "ZEE5",          232: "SonyLIV",          31: "HBO",
	350: "Apple TV",      43:  "Starz",
}

// Genre ID Map
var GenreMap = map[int]string{
	28: "Action", 12: "Adventure", 16: "Animation", 35: "Comedy",
	80: "Crime", 99: "Documentary", 18: "Drama", 10751: "Family",
	14: "Fantasy", 36: "History", 27: "Horror", 10402: "Music",
	9648: "Mystery", 10749: "Romance", 878: "Sci-Fi", 53: "Thriller",
	10752: "War", 37: "Western",
	10759: "Action & Adventure", 10762: "Kids", 10763: "News",
	10764: "Reality", 10765: "Sci-Fi & Fantasy", 10766: "Soap",
	10767: "Talk", 10768: "War & Politics",
}

func isIndianLanguage(lang string) bool {
	switch lang {
	case "hi", "te", "ta", "ml", "kn", "bn", "mr", "pa", "gu", "or", "ur", "as", "kok", "ne", "ks", "sd", "sa", "mai":
		return true
	}
	return false
}

func isIndianMovie(originalLanguage string, originCountry []string) bool {
	if isIndianLanguage(originalLanguage) {
		return true
	}
	for _, country := range originCountry {
		if country == "IN" {
			return true
		}
	}
	return false
}

func isYear2026(relDate string) bool {
	date := strings.TrimSpace(relDate)
	if date == "" {
		return false
	}
	if strings.EqualFold(date, "today") {
		return time.Now().Year() == 2026
	}
	if t, err := time.Parse("2006-01-02", date); err == nil {
		return t.Year() == 2026
	}
	if t, err := time.Parse("02/01/2006", date); err == nil {
		return t.Year() == 2026
	}
	if year, err := strconv.Atoi(date); err == nil {
		return year == 2026
	}
	if strings.Contains(date, "2026") {
		return true
	}
	return false
}

func verifyIndianMovieTMDB(ctx context.Context, client *http.Client, apiKey string, title string) (bool, string, string, float64, []int) {
	if apiKey == "" {
		return true, "", "", 0, nil
	}

	query := url.QueryEscape(title)
	u := fmt.Sprintf("%s/search/movie?api_key=%s&query=%s&language=en-US&page=1", tmdbBase, apiKey, query)

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return false, "", "", 0, nil
	}

	resp, err := client.Do(req)
	if err != nil {
		return false, "", "", 0, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, "", "", 0, nil
	}

	var data struct {
		Results []struct {
			ID               int      `json:"id"`
			Title            string   `json:"title"`
			ReleaseDate      string   `json:"release_date"`
			Overview         string   `json:"overview"`
			PosterPath       string   `json:"poster_path"`
			VoteAverage      float64  `json:"vote_average"`
			GenreIDs         []int    `json:"genre_ids"`
			OriginalLanguage string   `json:"original_language"`
			OriginCountry    []string `json:"origin_country"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return false, "", "", 0, nil
	}

	if len(data.Results) == 0 {
		return false, "", "", 0, nil
	}

	for i, item := range data.Results {
		if i > 2 {
			break
		}

		isIndian := false
		if isIndianLanguage(item.OriginalLanguage) {
			isIndian = true
		} else {
			for _, country := range item.OriginCountry {
				if country == "IN" {
					isIndian = true
					break
				}
			}
		}

		if !isIndian {
			continue
		}

		if !isYear2026(item.ReleaseDate) {
			continue
		}

		poster := ""
		if item.PosterPath != "" {
			poster = tmdbImg + item.PosterPath
		}
		return true, item.Overview, poster, item.VoteAverage, item.GenreIDs
	}

	return false, "", "", 0, nil
}

func getTMDBApiKey() string {
	return os.Getenv("TMDB_API_KEY")
}

func getJustWatchCountry() string {
	c := os.Getenv("JUSTWATCH_COUNTRY")
	if c == "" {
		return "IN"
	}
	return strings.ToUpper(c)
}

var spaceRegex = regexp.MustCompile(`\s+`)

func normalizeTitle(title string) string {
	t := strings.TrimSpace(strings.ToLower(title))
	return spaceRegex.ReplaceAllString(t, " ")
}

func normalizeReleaseDate(relDate string) string {
	date := strings.TrimSpace(relDate)
	if t, err := time.Parse("02/01/2006", date); err == nil {
		return t.Format("2006-01-02")
	}
	if t, err := time.Parse("2006-01-02", date); err == nil {
		return t.Format("2006-01-02")
	}
	return date
}

// FormatStars builds star symbols for a rating.
func FormatStars(r float64) string {
	if r >= 8 {
		return "⭐⭐⭐⭐⭐"
	}
	if r >= 6 {
		return "⭐⭐⭐⭐"
	}
	if r >= 4 {
		return "⭐⭐⭐"
	}
	if r >= 2 {
		return "⭐⭐"
	}
	return "⭐"
}

// FormatItemMessage formats a ReleaseItem into HTML text for Telegram.
func FormatItemMessage(item ReleaseItem) string {
	emoji := "🎬"
	kind := "Movie"
	if item.Type != "movie" {
		emoji = "📺"
		kind = "TV Series"
	}

	title := item.Title
	rel := item.ReleaseDate
	if rel == "" {
		rel = "N/A"
	}
	overview := item.Overview
	if overview == "" {
		overview = "No description."
	}
	if len(overview) > 600 {
		overview = overview[:597] + "..."
	}

	provs := "Check JustWatch"
	if len(item.Providers) > 0 {
		provs = strings.Join(item.Providers, " • ")
	}

	var genres []string
	for _, gid := range item.GenreIDs {
		if name, ok := GenreMap[gid]; ok {
			genres = append(genres, name)
		}
	}
	genres = append(genres, item.GenreNames...)
	genreS := ""
	if len(genres) > 0 {
		// Unique genres
		uniqueGenres := []string{}
		seen := make(map[string]bool)
		for _, g := range genres {
			if !seen[g] {
				seen[g] = true
				uniqueGenres = append(uniqueGenres, g)
			}
		}
		if len(uniqueGenres) > 3 {
			uniqueGenres = uniqueGenres[:3]
		}
		genreS = strings.Join(uniqueGenres, " | ")
	}

	tmdbURL := ""
	if item.Source == "tmdb" {
		mpath := "movie"
		if item.Type != "movie" {
			mpath = "tv"
		}
		tmdbURL = fmt.Sprintf("https://www.themoviedb.org/%s/%v", mpath, item.ID)
	}
	country := strings.ToLower(getJustWatchCountry())
	jwURL := fmt.Sprintf("https://www.justwatch.com/%s/%s", country, "movie")
	if item.Type != "movie" {
		jwURL = fmt.Sprintf("https://www.justwatch.com/%s/%s", country, "tv-show")
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("%s <b>%s</b>  <code>[%s]</code>", emoji, title, kind))
	lines = append(lines, fmt.Sprintf("📅 <b>Released:</b> %s", rel))
	if item.Rating > 0 {
		lines = append(lines, fmt.Sprintf("⭐ <b>Rating:</b> %.1f/10  %s", item.Rating, FormatStars(item.Rating)))
	}
	if genreS != "" {
		lines = append(lines, fmt.Sprintf("🎭 <b>Genre:</b> %s", genreS))
	}
	lines = append(lines, fmt.Sprintf("📡 <b>Available on:</b> %s", provs))
	lines = append(lines, "")
	lines = append(lines, overview)

	if tmdbURL != "" {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf(`<a href="%s">📖 TMDB</a>  |  <a href="%s">🍿 JustWatch</a>`, tmdbURL, jwURL))
	} else {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf(`<a href="%s">🍿 JustWatch</a>`, jwURL))
	}

	return strings.Join(lines, "\n")
}

// FormatItemKeyboard creates inline buttons for a release item.
func FormatItemKeyboard(item ReleaseItem) gotgbot.InlineKeyboardMarkup {
	country := strings.ToLower(getJustWatchCountry())
	mpath := "movie"
	if item.Type != "movie" {
		mpath = "tv-show"
	}
	jwURL := fmt.Sprintf("https://www.justwatch.com/%s/%s", country, mpath)

	var rows [][]gotgbot.InlineKeyboardButton
	rows = append(rows, []gotgbot.InlineKeyboardButton{
		{Text: "▶️ Open JustWatch", Url: jwURL},
	})

	if item.Source == "tmdb" {
		tmpath := "movie"
		if item.Type != "movie" {
			tmpath = "tv"
		}
		tmdbURL := fmt.Sprintf("https://www.themoviedb.org/%s/%v", tmpath, item.ID)
		rows = append(rows, []gotgbot.InlineKeyboardButton{
			{Text: "🎬 Open TMDB", Url: tmdbURL},
		})
	}

	return gotgbot.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// Fetch TMDB Watch Providers
func _fetchTMDBProviders(ctx context.Context, client *http.Client, mediaType string, tmdbID int, apiKey, country string) ([]string, error) {
	u := fmt.Sprintf("%s/%s/%d/watch/providers?api_key=%s", tmdbBase, mediaType, tmdbID, apiKey)
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TMDB Watch Providers status %d", resp.StatusCode)
	}

	var data TMDBProvidersResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	region, ok := data.Results[country]
	if !ok {
		return nil, nil
	}

	seen := make(map[string]bool)
	var list []string
	for _, p := range region.Flatrate {
		name := OTTProviders[p.ProviderID]
		if name == "" {
			name = p.ProviderName
		}
		if name != "" && !seen[name] {
			seen[name] = true
			list = append(list, name)
		}
	}
	for _, p := range region.Free {
		name := OTTProviders[p.ProviderID]
		if name == "" {
			name = p.ProviderName
		}
		if name != "" && !seen[name] {
			seen[name] = true
			list = append(list, name)
		}
	}
	for _, p := range region.Ads {
		name := OTTProviders[p.ProviderID]
		if name == "" {
			name = p.ProviderName
		}
		if name != "" && !seen[name] {
			seen[name] = true
			list = append(list, name)
		}
	}

	return list, nil
}

type TMDBProvidersResponse struct {
	Results map[string]struct {
		Flatrate []struct {
			ProviderID   int    `json:"provider_id"`
			ProviderName string `json:"provider_name"`
		} `json:"flatrate"`
		Free []struct {
			ProviderID   int    `json:"provider_id"`
			ProviderName string `json:"provider_name"`
		} `json:"free"`
		Ads []struct {
			ProviderID   int    `json:"provider_id"`
			ProviderName string `json:"provider_name"`
		} `json:"ads"`
	} `json:"results"`
}

// Discover TMDB Releases
func _fetchTMDB(ctx context.Context, client *http.Client, daysBack int, db database.Database, dedup bool) ([]ReleaseItem, error) {
	apiKey := getTMDBApiKey()
	country := getJustWatchCountry()
	since := time.Now().AddDate(0, 0, -daysBack).Format("2006-01-02")
	today := time.Now().Format("2006-01-02")

	var results []ReleaseItem

	mt := "movie"
	dateField := "primary_release_date"

	u := fmt.Sprintf("%s/discover/%s?api_key=%s&language=en-US&sort_by=popularity.desc&with_watch_monetization_types=flatrate|free|ads&watch_region=%s&%s.gte=%s&%s.lte=%s&with_origin_country=IN&page=1",
		tmdbBase, mt, apiKey, country, dateField, since, dateField, today)

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	var data TMDBDiscoverResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		resp.Body.Close()
		return nil, err
	}
	resp.Body.Close()

	for _, item := range data.Results {
		itemID := fmt.Sprintf("tmdb_%s_%d", mt, item.ID)
		if dedup {
			if sent, _ := db.IsOTTItemSent(itemID); sent {
				continue
			}
		}

		// Verify it's a 2026 year movie
		if !isYear2026(item.ReleaseDate) {
			continue
		}

		// Verify it's an Indian movie
		if !isIndianMovie(item.OriginalLanguage, item.OriginCountry) {
			continue
		}

		// Fetch watch providers
		provs, err := _fetchTMDBProviders(ctx, client, mt, item.ID, apiKey, country)
		if err != nil || len(provs) == 0 {
			continue
		}

		title := item.Title
		relDate := item.ReleaseDate
		poster := ""
		if item.PosterPath != "" {
			poster = tmdbImg + item.PosterPath
		}

		if dedup {
			_ = db.MarkOTTItemSent(itemID, title)
		}

		var genres []string
		for _, gid := range item.GenreIDs {
			if name, ok := GenreMap[gid]; ok {
				genres = append(genres, name)
			}
		}

		results = append(results, ReleaseItem{
			ID:          item.ID,
			ItemID:      itemID,
			Type:        mt,
			Title:       title,
			ReleaseDate: relDate,
			Overview:    item.Overview,
			Providers:   provs,
			Poster:      poster,
			Rating:      item.VoteAverage,
			GenreIDs:    item.GenreIDs,
			GenreNames:  genres,
			Source:      "tmdb",
		})
	}

	return results, nil
}

type TMDBDiscoverResponse struct {
	Results []struct {
		ID               int      `json:"id"`
		Title            string   `json:"title"`
		Name             string   `json:"name"`
		ReleaseDate      string   `json:"release_date"`
		FirstAirDate     string   `json:"first_air_date"`
		Overview         string   `json:"overview"`
		PosterPath       string   `json:"poster_path"`
		VoteAverage      float64  `json:"vote_average"`
		GenreIDs         []int    `json:"genre_ids"`
		OriginalLanguage string   `json:"original_language"`
		OriginCountry    []string `json:"origin_country"`
	} `json:"results"`
}

// GraphQL Query
const jwGQLQuery = `
query GetPopularTitles($country: Country!, $language: Language!) {
  popularTitles(
    country: $country
    first: 200
    sortBy: POPULAR
    filter: {
      objectTypes: [MOVIE, SHOW]
      monetizationTypes: [FLATRATE, FREE, ADS]
    }
  ) {
    edges {
      node {
        id
        objectType
        content(country: $country, language: $language) {
          title
          shortDescription
          originalReleaseYear
          posterUrl
        }
        offers(
          country: $country
          platform: WEB
          filter: { monetizationTypes: [FLATRATE, FREE, ADS] }
        ) {
          package { clearName shortName }
        }
      }
    }
  }
}
`

type JWGQLResponse struct {
	Data struct {
		PopularTitles struct {
			Edges []struct {
				Node struct {
					ID         string `json:"id"`
					ObjectType string `json:"objectType"`
					Content    struct {
						Title               string `json:"title"`
						ShortDescription    string `json:"shortDescription"`
						OriginalReleaseYear int    `json:"originalReleaseYear"`
						PosterURL           string `json:"posterUrl"`
					} `json:"content"`
					Offers []struct {
						Package struct {
							ClearName string `json:"clearName"`
							ShortName string `json:"shortName"`
						} `json:"package"`
					} `json:"offers"`
				} `json:"node"`
			} `json:"edges"`
		} `json:"popularTitles"`
	} `json:"data"`
}

// Fetch JustWatch GraphQL Fallback
func _fetchJustWatch(ctx context.Context, client *http.Client, daysBack int, db database.Database, dedup bool) ([]ReleaseItem, error) {
	country := getJustWatchCountry()
	payload := map[string]interface{}{
		"query": jwGQLQuery,
		"variables": map[string]interface{}{
			"country":  country,
			"language": "en",
		},
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", jwGqlURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; OTTBot/3.0)")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("JustWatch GQL status %d", resp.StatusCode)
	}

	var data JWGQLResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	apiKey := getTMDBApiKey()
	var results []ReleaseItem
	for _, edge := range data.Data.PopularTitles.Edges {
		node := edge.Node
		if node.ObjectType != "MOVIE" {
			continue
		}

		// Must be 2026
		if node.Content.OriginalReleaseYear != 2026 {
			continue
		}

		// Verify on TMDB that it's Indian movie and released in 2026
		isIndian, overview, poster, rating, genres := verifyIndianMovieTMDB(ctx, client, apiKey, node.Content.Title)
		if !isIndian {
			continue
		}

		var providers []string
		for _, offer := range node.Offers {
			name := offer.Package.ClearName
			if name == "" {
				name = offer.Package.ShortName
			}
			if name != "" {
				// Dedup
				found := false
				for _, p := range providers {
					if p == name {
						found = true
						break
					}
				}
				if !found {
					providers = append(providers, name)
				}
			}
		}

		if len(providers) == 0 {
			continue
		}

		itemID := fmt.Sprintf("jw_%s", node.ID)
		if dedup {
			if sent, _ := db.IsOTTItemSent(itemID); sent {
				continue
			}
		}

		finalOverview := node.Content.ShortDescription
		if overview != "" {
			finalOverview = overview
		}
		finalPoster := node.Content.PosterURL
		if finalPoster != "" {
			finalPoster = strings.ReplaceAll(finalPoster, "{profile}", "s592")
			finalPoster = strings.ReplaceAll(finalPoster, "{format}", "jpg")
			if strings.HasPrefix(finalPoster, "/") {
				finalPoster = "https://images.justwatch.com" + finalPoster
			}
		}
		if poster != "" {
			finalPoster = poster
		}

		var genreNames []string
		for _, gid := range genres {
			if name, ok := GenreMap[gid]; ok {
				genreNames = append(genreNames, name)
			}
		}

		if dedup {
			_ = db.MarkOTTItemSent(itemID, node.Content.Title)
		}

		results = append(results, ReleaseItem{
			ID:          node.ID,
			ItemID:      itemID,
			Type:        "movie",
			Title:       node.Content.Title,
			ReleaseDate: fmt.Sprintf("%d", node.Content.OriginalReleaseYear),
			Overview:    finalOverview,
			Providers:   providers,
			Poster:      finalPoster,
			Rating:      rating,
			GenreIDs:    genres,
			GenreNames:  genreNames,
			Source:      "justwatch",
		})
	}

	return results, nil
}

// Fetch JustWatch Web Scraper Fallback
func _fetchJustWatchWeb(ctx context.Context, client *http.Client, db database.Database, dedup bool) ([]ReleaseItem, error) {
	country := strings.ToLower(getJustWatchCountry())
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("https://www.justwatch.com/%s/new", country), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/115.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("JustWatch web scrape status %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	apiKey := getTMDBApiKey()
	var results []ReleaseItem

	doc.Find("div.timeline__provider-block").Each(func(i int, block *goquery.Selection) {
		provider := ""
		logo := block.Find(".provider-timeline__logo img")
		if logo.Length() > 0 {
			provider = strings.TrimSpace(logo.AttrOr("alt", ""))
		}
		if provider == "" {
			provider = "Unknown"
		}

		block.Find(".horizontal-title-list__item").Each(func(j int, item *goquery.Selection) {
			a := item.Find("a[href]")
			img := item.Find(".title-poster__image img")
			if a.Length() == 0 || img.Length() == 0 {
				return
			}
			href := a.AttrOr("href", "")
			title := strings.TrimSpace(img.AttrOr("alt", ""))
			poster := strings.TrimSpace(img.AttrOr("src", ""))

			if strings.Contains(href, "/tv-show/") {
				return
			}

			// Verify on TMDB that it's Indian movie and released in 2026
			isIndian, overview, tmdbPoster, rating, genres := verifyIndianMovieTMDB(ctx, client, apiKey, title)
			if !isIndian {
				return
			}

			itemID := fmt.Sprintf("jw_web_%s", href)
			if dedup {
				if sent, _ := db.IsOTTItemSent(itemID); sent {
					return
				}
			}

			if tmdbPoster != "" {
				poster = tmdbPoster
			}

			var genreNames []string
			for _, gid := range genres {
				if name, ok := GenreMap[gid]; ok {
					genreNames = append(genreNames, name)
				}
			}

			if dedup {
				_ = db.MarkOTTItemSent(itemID, title)
			}

			results = append(results, ReleaseItem{
				ID:          href,
				ItemID:      itemID,
				Type:        "movie",
				Title:       title,
				ReleaseDate: fmt.Sprintf("%d", time.Now().Year()),
				Overview:    overview,
				Providers:   []string{provider},
				Poster:      poster,
				Rating:      rating,
				GenreIDs:    genres,
				GenreNames:  genreNames,
				Source:      "justwatch_web",
			})
		})
	})

	return results, nil
}

// Fetch OTTRelease Scraper
func _fetchOTTRelease(ctx context.Context, client *http.Client, db database.Database, dedup bool) ([]ReleaseItem, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", ottreleaseURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OTTRelease status %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	type rawLink struct {
		Href, Title, Date, Poster string
	}
	var links []rawLink

	doc.Find("div.streaming-now-grid div.stream-item a[href]").Each(func(i int, a *goquery.Selection) {
		href := strings.TrimSpace(a.AttrOr("href", ""))
		titleEl := a.Find(".stream-title")
		dateEl := a.Find(".stream-date")
		title := strings.TrimSpace(titleEl.Text())
		rawDate := strings.TrimSpace(dateEl.Text())
		rawDate = strings.TrimSpace(strings.ReplaceAll(rawDate, "Released:", ""))
		img := a.Find("img")
		poster := strings.TrimSpace(img.AttrOr("src", ""))

		if href != "" && title != "" {
			links = append(links, rawLink{href, title, rawDate, poster})
		}
	})

	apiKey := getTMDBApiKey()
	var mu sync.Mutex
	var results []ReleaseItem
	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup

	for _, link := range links {
		wg.Add(1)
		go func(lnk rawLink) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if strings.Contains(lnk.Href, "/web-series/") || strings.Contains(lnk.Href, "/tv-show/") {
				return
			}

			// Verify on TMDB that it's Indian movie and released in 2026
			isIndian, tmdbOverview, tmdbPoster, rating, genres := verifyIndianMovieTMDB(ctx, client, apiKey, lnk.Title)
			if !isIndian {
				return
			}

			providers := []string{}
			overview := "No description."

			// Request detail page
			dreq, err := http.NewRequestWithContext(ctx, "GET", lnk.Href, nil)
			if err == nil {
				dreq.Header.Set("User-Agent", "Mozilla/5.0")
				dresp, err := client.Do(dreq)
				if err == nil {
					defer dresp.Body.Close()
					if dresp.StatusCode == http.StatusOK {
						ddoc, err := goquery.NewDocumentFromReader(dresp.Body)
						if err == nil {
							ddoc.Find(".ott-badge a").Each(func(idx int, s *goquery.Selection) {
								p := strings.TrimSpace(s.Text())
								if p != "" {
									found := false
									for _, exist := range providers {
										if exist == p {
											found = true
											break
										}
									}
									if !found {
										providers = append(providers, p)
									}
								}
							})

							storyP := ddoc.Find(".movie-description p").Last()
							if storyP.Length() > 0 {
								overview = strings.TrimSpace(storyP.Text())
							}
						}
					}
				}
			}

			parts := strings.Split(strings.TrimRight(lnk.Href, "/"), "/")
			lastPart := parts[len(parts)-1]
			itemID := fmt.Sprintf("ottrelease_%s", lastPart)

			if dedup {
				if sent, _ := db.IsOTTItemSent(itemID); sent {
					return
				}
			}

			if tmdbOverview != "" {
				overview = tmdbOverview
			}
			if tmdbPoster != "" {
				lnk.Poster = tmdbPoster
			}

			var genreNames []string
			for _, gid := range genres {
				if name, ok := GenreMap[gid]; ok {
					genreNames = append(genreNames, name)
				}
			}

			if dedup {
				_ = db.MarkOTTItemSent(itemID, lnk.Title)
			}

			mu.Lock()
			results = append(results, ReleaseItem{
				ID:          lnk.Href,
				ItemID:      itemID,
				Type:        "movie",
				Title:       lnk.Title,
				ReleaseDate: lnk.Date,
				Overview:    overview,
				Providers:   providers,
				Poster:      lnk.Poster,
				Rating:      rating,
				GenreIDs:    genres,
				GenreNames:  genreNames,
				Source:      "ottrelease",
			})
			mu.Unlock()
		}(link)
	}
	wg.Wait()

	return results, nil
}

// GetNewReleases coordinates fetching from TMDB / JustWatch / OTTRelease
func GetNewReleases(ctx context.Context, db database.Database, daysBack int, dedup bool) ([]ReleaseItem, error) {
	client := &http.Client{
		Timeout: 20 * time.Second,
	}

	var items []ReleaseItem
	var err error

	apiKey := getTMDBApiKey()
	if apiKey != "" {
		items, err = _fetchTMDB(ctx, client, daysBack, db, dedup)
		if err != nil || len(items) == 0 {
			items, _ = _fetchJustWatchWeb(ctx, client, db, dedup)
			if len(items) == 0 {
				items, _ = _fetchJustWatch(ctx, client, daysBack, db, dedup)
			}
		}
	} else {
		items, _ = _fetchJustWatchWeb(ctx, client, db, dedup)
		if len(items) == 0 {
			items, _ = _fetchJustWatch(ctx, client, daysBack, db, dedup)
		}
	}

	ottreleaseItems, _ := _fetchOTTRelease(ctx, client, db, dedup)
	all := append(items, ottreleaseItems...)

	// Merge & Dedup by normalized title and normalized release date
	var merged []ReleaseItem
	seen := make(map[string]bool)
	for _, it := range all {
		key := fmt.Sprintf("%s|%s", normalizeTitle(it.Title), normalizeReleaseDate(it.ReleaseDate))
		if seen[key] {
			continue
		}
		seen[key] = true
		merged = append(merged, it)
	}

	return merged, nil
}
