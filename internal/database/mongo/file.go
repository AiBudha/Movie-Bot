package mongo

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"autofilterbot/internal/database"
	"autofilterbot/internal/functions"
	"autofilterbot/internal/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

func (c *Client) SaveFile(f *model.File) error {
	// Find any with matching file_id
	if res := c.fileCollection.FindOne(c.ctx, fileIdFilter(f.FileId)); res.Err() != mongo.ErrNoDocuments {
		return database.FileAlreadyExistsError{FileName: f.FileName}
	}

	// Find a document with the same file_name (case-insensitive) and within a 100 byte range of file_size
	duplicateFilter := bson.M{
		"file_name": bson.M{"$regex": "^" + regexp.QuoteMeta(f.FileName) + "$", "$options": "i"},
		"file_size": bson.M{
			"$gte": f.FileSize - 100,
			"$lte": f.FileSize + 100,
		},
	}
	if res := c.fileCollection.FindOne(c.ctx, duplicateFilter); res.Err() != mongo.ErrNoDocuments {
		return database.FileAlreadyExistsError{FileName: f.FileName}
	}

	_, err := c.fileCollection.InsertOne(c.ctx, f)

	return err
}

func (c *Client) SaveFiles(files ...*model.File) []error {
	var errs []error

	for _, f := range files {
		if err := c.SaveFile(f); err != nil {
			errs = append(errs, err)
		}
	}

	return errs
}

func (c *Client) BulkSaveFiles(files []*model.File) error {
	if len(files) == 0 {
		return nil
	}

	// 1. Deduplicate the input list amongst itself
	seenFileId := make(map[string]bool)
	type nameSize struct {
		name string
		size int64
	}
	var seenNameSizes []nameSize
	var uniqueFiles []*model.File

	for _, f := range files {
		if f.FileId == "" || seenFileId[f.FileId] {
			continue
		}

		hasInMemoryDup := false
		for _, sns := range seenNameSizes {
			if strings.EqualFold(sns.name, f.FileName) && f.FileSize >= sns.size-100 && f.FileSize <= sns.size+100 {
				hasInMemoryDup = true
				break
			}
		}
		if hasInMemoryDup {
			continue
		}

		seenFileId[f.FileId] = true
		seenNameSizes = append(seenNameSizes, nameSize{name: f.FileName, size: f.FileSize})
		uniqueFiles = append(uniqueFiles, f)
	}

	if len(uniqueFiles) == 0 {
		return nil
	}

	// 2. Query the database using $or to find which ones already exist
	var orFilters []bson.M
	for _, f := range uniqueFiles {
		orFilters = append(orFilters, bson.M{"file_id": f.FileId})
		orFilters = append(orFilters, bson.M{
			"file_name": bson.M{"$regex": "^" + regexp.QuoteMeta(f.FileName) + "$", "$options": "i"},
			"file_size": bson.M{
				"$gte": f.FileSize - 100,
				"$lte": f.FileSize + 100,
			},
		})
	}

	existingFileIds := make(map[string]bool)
	existingNameSizes := make(map[string][]int64)

	cursor, err := c.fileCollection.Find(c.ctx, bson.M{"$or": orFilters})
	if err == nil {
		defer cursor.Close(c.ctx)
		for cursor.Next(c.ctx) {
			var dbf model.File
			if err := cursor.Decode(&dbf); err == nil {
				if dbf.FileId != "" {
					existingFileIds[dbf.FileId] = true
				}
				if dbf.FileName != "" {
					lowerName := strings.ToLower(dbf.FileName)
					existingNameSizes[lowerName] = append(existingNameSizes[lowerName], dbf.FileSize)
				}
			}
		}
	}

	// 3. Construct Bulk Write Models only for files that DO NOT exist in DB
	var models []mongo.WriteModel
	for _, f := range uniqueFiles {
		hasDup := false
		lowerName := strings.ToLower(f.FileName)
		if sizes, ok := existingNameSizes[lowerName]; ok {
			for _, sz := range sizes {
				if sz >= f.FileSize-100 && sz <= f.FileSize+100 {
					hasDup = true
					break
				}
			}
		}
		if existingFileIds[f.FileId] || hasDup {
			continue
		}

		filter := bson.D{{Key: "file_id", Value: f.FileId}}
		upsert := true
		models = append(models, mongo.NewUpdateOneModel().
			SetFilter(filter).
			SetUpdate(bson.D{{Key: "$setOnInsert", Value: f}}).
			SetUpsert(upsert))
	}

	if len(models) == 0 {
		return nil
	}

	opts := options.BulkWrite().SetOrdered(false)
	_, err = c.fileCollection.BulkWrite(c.ctx, models, opts)
	return err
}

func (c *Client) GetFile(fileId string) (*model.File, error) {
	res := c.fileCollection.FindOne(c.ctx, idFilter(fileId))
	if err := res.Err(); err != nil {
		return nil, err
	}

	var f model.File

	err := res.Decode(&f)

	return &f, err
}

func (c *Client) DeleteFile(fileId string) error {
	_, err := c.fileCollection.DeleteOne(c.ctx, idFilter(fileId))
	return err
}

type sliceCursor struct {
	files []*model.File
	index int
}

func (s *sliceCursor) Next(ctx context.Context) bool {
	s.index++
	return s.index < len(s.files)
}

func (s *sliceCursor) Decode(v interface{}) error {
	if s.index < 0 || s.index >= len(s.files) {
		return mongo.ErrNoDocuments
	}
	if ptr, ok := v.(*model.File); ok {
		*ptr = *s.files[s.index]
		return nil
	}
	return fmt.Errorf("sliceCursor: unsupported decode target type %T", v)
}

func (s *sliceCursor) Close(ctx context.Context) error {
	return nil
}

func (c *Client) SearchFiles(query string) (database.Cursor, error) {
	query = strings.ReplaceAll(query, ".", " ")
	query = strings.ReplaceAll(query, "-", " ")
	query = strings.ReplaceAll(query, "_", " ")
	query = strings.ReplaceAll(query, "+", " ")

	words := strings.Fields(query)
	if len(words) == 0 {
		return nil, mongo.ErrNilDocument
	}

	languageAliases := map[string]string{
		"hindi":     "(hindi|hin)",
		"english":   "(english|eng)",
		"tamil":     "(tamil|tam)",
		"telugu":    "(telugu|tel)",
		"malayalam": "(malayalam|mal)",
		"kannada":   "(kannada|kan)",
		"multi":     "(multi|dual|mux)",
	}

	descriptiveCount := 0
	var nonDescriptiveRegex = regexp.MustCompile(`(?i)^\b(19\d\d|20\d\d|\d{3,4}p|4k|bluray|webrip|web|hdrip|brrip|hdtv|x264|x265|hevc|h264|h265|camrip|dd5\.1|dual|multi|hindi|english|tamil|telugu|malayalam|kannada|bengali|marathi|bhojpuri|punjabi|gujarati)\b$`)
	for _, word := range words {
		if !nonDescriptiveRegex.MatchString(word) && len(word) > 2 {
			descriptiveCount++
		}
	}

	var searchWords []string
	// Compile regexes for Go-side filtering (avoids slow MongoDB lookahead scans)
	var goRegexes []*regexp.Regexp
	for _, word := range words {
		lowerWord := strings.ToLower(word)
		isNonDescriptive := nonDescriptiveRegex.MatchString(word)

		if aliasGroup, ok := languageAliases[lowerWord]; ok {
			if descriptiveCount <= 0 {
				searchWords = append(searchWords,
					strings.ReplaceAll(strings.ReplaceAll(aliasGroup, "(", ""), ")", ""),
					strings.ReplaceAll(aliasGroup, "|", " "),
				)
			}
			re, err := regexp.Compile(fmt.Sprintf("(?i)(?:[^a-zA-Z0-9]|^)(?:%s)(?:[^a-zA-Z0-9]|$)", aliasGroup))
			if err == nil {
				goRegexes = append(goRegexes, re)
			}
		} else {
			if !(descriptiveCount > 0 && isNonDescriptive) {
				searchWords = append(searchWords, word)
			}
			quotedWord := regexp.QuoteMeta(word)
			var rePattern string
			if matched, _ := regexp.MatchString("(?i)^s\\d+$", word); matched {
				rePattern = fmt.Sprintf("(?i)(?:[^a-zA-Z0-9]|^)%s(?:[eE]\\d+|[^a-zA-Z0-9]|$)", quotedWord)
			} else {
				rePattern = fmt.Sprintf("(?i)(?:[^a-zA-Z0-9]|^)%s(?:[^a-zA-Z0-9]|$)", quotedWord)
			}
			re, err := regexp.Compile(rePattern)
			if err == nil {
				goRegexes = append(goRegexes, re)
			}
		}
	}
	searchTerms := strings.Join(searchWords, " ")

	// --- Phase 1: Text-index search only (no slow regex on Atlas) ---
	// We fetch up to 1000 candidates using only the fast text index,
	// then filter them in Go using pre-compiled regexes. This is ~8.5x faster
	// than sending lookahead regex patterns to MongoDB Atlas.
	textOnlyPipeline := bson.D{
		{Key: "$text", Value: bson.D{{Key: "$search", Value: searchTerms}}},
	}

	var files []*model.File

	filterByGoRegex := func(candidates []*model.File) []*model.File {
		if len(goRegexes) == 0 {
			return candidates
		}
		var matched []*model.File
		for _, f := range candidates {
			ok := true
			for _, re := range goRegexes {
				if !re.MatchString(f.FileName) {
					ok = false
					break
				}
			}
			if ok {
				matched = append(matched, f)
			}
		}
		return matched
	}

	ctx1, cancel1 := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel1()
	cursor, err := c.fileCollection.Find(ctx1, textOnlyPipeline, options.Find().SetSort(bson.M{"time": -1}).SetLimit(1000))
	if err == nil {
		defer cursor.Close(ctx1)
		var candidates []*model.File
		for cursor.Next(ctx1) {
			var f model.File
			if err := cursor.Decode(&f); err == nil {
				candidates = append(candidates, &f)
			}
		}
		files = filterByGoRegex(candidates)
	}

	return &sliceCursor{files: files, index: -1}, nil
}

// fileIdFilter creates a bson filter to match by file_id.
func fileIdFilter(id string) bson.D {
	return bson.D{{Key: "file_id", Value: id}}
}

var (
	// regex to match year or season or quality indicators
	cutRegex = regexp.MustCompile(`(?i)\b(19\d\d|20\d\d|s\d+e\d+|s\d+|e\d+|\d+p|webrip|web-rip|bluray|hdrip|brrip|camrip|hdtv|x264|x265|hevc|10bit)\b`)
)

func extractTitle(fileName string) (string, string) {
	// Remove promotional prefixes first
	fileName = functions.CleanPromoFromName(fileName)

	// Remove extension
	if idx := strings.LastIndex(fileName, "."); idx != -1 {
		ext := fileName[idx:]
		if len(ext) <= 5 {
			fileName = fileName[:idx]
		}
	}
	
	// Replace separators with spaces
	r := regexp.MustCompile(`[\.\-_\(\)\[\]\{\}\+\*]`)
	cleaned := r.ReplaceAllString(fileName, " ")
	
	// Find cut point
	loc := cutRegex.FindStringSubmatchIndex(cleaned)
	title := cleaned
	year := ""
	if len(loc) >= 2 {
		cutPos := loc[0]
		title = cleaned[:cutPos]
		
		// If the match was a year, extract it
		match := cleaned[loc[0]:loc[1]]
		if regexp.MustCompile(`^(19\d\d|20\d\d)$`).MatchString(match) {
			year = match
		}
	}
	
	// Clean extra spaces
	titleFields := strings.Fields(title)
	title = strings.Join(titleFields, " ")
	
	// Strip trailing languages/audio tags from title
	langRegex := regexp.MustCompile(`(?i)\s+\b(hindi|hin|english|eng|tamil|tam|telugu|tel|malayalam|mal|kannada|kan|multi|dual|mux|dubbed|dub|org|original|sub|subs|esub|esubs|hqc|hq|clean|hevc|x264|x265|av1|rip|web-dl|webrip|hdr|10bit)\b\s*$`)
	for {
		old := title
		title = langRegex.ReplaceAllString(title, "")
		title = strings.TrimSpace(title)
		if title == old {
			break
		}
	}
	
	return title, year
}

func getFuzzyTerms(word string) []string {
	if len(word) < 3 {
		return []string{word}
	}
	runes := []rune(word)
	n := len(runes)

	variations := make(map[string]bool)
	variations[word] = true

	// Deletions
	for i := 0; i < n; i++ {
		temp := make([]rune, 0, n-1)
		temp = append(temp, runes[:i]...)
		temp = append(temp, runes[i+1:]...)
		variations[string(temp)] = true
	}

	// Transpositions
	for i := 0; i < n-1; i++ {
		temp := make([]rune, n)
		copy(temp, runes)
		temp[i], temp[i+1] = temp[i+1], temp[i]
		variations[string(temp)] = true
	}

	res := make([]string, 0, len(variations))
	for k := range variations {
		if len(k) >= 3 {
			res = append(res, k)
		}
	}
	return res
}

func (c *Client) GetSpellingSuggestions(query string) ([]string, error) {
	query = strings.ReplaceAll(query, ".", " ")
	query = strings.ReplaceAll(query, "-", " ")
	query = strings.ReplaceAll(query, "_", " ")
	query = strings.ReplaceAll(query, "+", " ")

	words := strings.Fields(query)
	if len(words) == 0 {
		return nil, nil
	}

	maxAllowedDist := len(query) / 2
	if maxAllowedDist < 3 {
		maxAllowedDist = 3
	}

	type candidate struct {
		title    string
		distance int
		year     string
	}

	var suggestions []string
	seenSugs := make(map[string]bool)

	addSuggestionsFromFiles := func(files []*model.File) {
		var cands []candidate
		seenTitles := make(map[string]bool)

		for _, f := range files {
			title, year := extractTitle(f.FileName)
			lowerTitle := strings.ToLower(title)
			if seenTitles[lowerTitle] {
				continue
			}
			seenTitles[lowerTitle] = true

			dist := functions.Levenshtein(lowerTitle, strings.ToLower(query))
			if dist <= maxAllowedDist {
				cands = append(cands, candidate{
					title:    title,
					distance: dist,
					year:     year,
				})
			}
		}

		sort.Slice(cands, func(i, j int) bool {
			if cands[i].distance != cands[j].distance {
				return cands[i].distance < cands[j].distance
			}
			return cands[i].year > cands[j].year
		})

		for _, cand := range cands {
			sug := cand.title
			if cand.year != "" {
				sug = fmt.Sprintf("%s (%s)", cand.title, cand.year)
			}
			lowerSug := strings.ToLower(sug)
			if !seenSugs[lowerSug] {
				seenSugs[lowerSug] = true
				suggestions = append(suggestions, sug)
			}
		}
	}

	// 1. Try Text Search first (fully indexed, extremely fast!)
	searchTerms := strings.Join(words, " ")
	textFilter := bson.D{{Key: "$text", Value: bson.D{{Key: "$search", Value: searchTerms}}}}
	ctx1, cancel1 := context.WithTimeout(context.Background(), 2*time.Second)
	cursor, err := c.fileCollection.Find(ctx1, textFilter, options.Find().SetLimit(100))
	if err == nil {
		var files1 []*model.File
		for cursor.Next(ctx1) {
			var f model.File
			if err := cursor.Decode(&f); err == nil {
				files1 = append(files1, &f)
			}
		}
		cursor.Close(ctx1)
		addSuggestionsFromFiles(files1)
	}
	cancel1()

	// 2. Fallback: Try fuzzy variations text search if we have fewer than 5 suggestions
	if len(suggestions) < 5 {
		var fuzzyTerms []string
		for _, word := range words {
			if len(word) >= 3 {
				fuzzyTerms = append(fuzzyTerms, getFuzzyTerms(word)...)
			} else {
				fuzzyTerms = append(fuzzyTerms, word)
			}
		}
		if len(fuzzyTerms) > 0 {
			fuzzySearch := strings.Join(fuzzyTerms, " ")
			textFilterFuzzy := bson.D{{Key: "$text", Value: bson.D{{Key: "$search", Value: fuzzySearch}}}}
			ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
			cursorFuzzy, errFuzzy := c.fileCollection.Find(ctx2, textFilterFuzzy, options.Find().SetLimit(100))
			if errFuzzy == nil {
				var files2 []*model.File
				for cursorFuzzy.Next(ctx2) {
					var f model.File
					if err := cursorFuzzy.Decode(&f); err == nil {
						files2 = append(files2, &f)
					}
				}
				cursorFuzzy.Close(ctx2)
				addSuggestionsFromFiles(files2)
			}
			cancel2()
		}
	}

	// 3. Regex Substring/Prefix fallback if we still have fewer than 5 suggestions
	if len(suggestions) < 5 {
		var regexFilters []bson.M
		for _, word := range words {
			if len(word) >= 3 {
				// Take the first half of the word (at least 3 characters)
				prefixLen := len(word) / 2
				if prefixLen < 3 {
					prefixLen = 3
				}
				if prefixLen > len(word) {
					prefixLen = len(word)
				}
				prefix := word[:prefixLen]
				regexFilters = append(regexFilters, bson.M{"file_name": bson.M{"$regex": "(?i)" + regexp.QuoteMeta(prefix)}})
			}
		}
		if len(regexFilters) > 0 {
			ctx3, cancel3 := context.WithTimeout(context.Background(), 2*time.Second)
			cursorRegex, errRegex := c.fileCollection.Find(ctx3, bson.M{"$and": regexFilters}, options.Find().SetLimit(500))
			if errRegex == nil {
				var files3 []*model.File
				for cursorRegex.Next(ctx3) {
					var f model.File
					if err := cursorRegex.Decode(&f); err == nil {
						files3 = append(files3, &f)
					}
				}
				cursorRegex.Close(ctx3)
				addSuggestionsFromFiles(files3)
			}
			cancel3()
		}
	}

	if len(suggestions) > 5 {
		suggestions = suggestions[:5]
	}

	return suggestions, nil
}

func (c *Client) GetRecent2026Files(limit int) ([]*model.File, error) {
	filter := bson.M{
		"file_name": bson.M{"$regex": `(?i)\b2026\b`},
	}
	opts := options.Find().SetSort(bson.M{"time": -1}).SetLimit(int64(limit))
	cursor, err := c.fileCollection.Find(c.ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(c.ctx)

	var files []*model.File
	for cursor.Next(c.ctx) {
		var f model.File
		if err := cursor.Decode(&f); err == nil {
			files = append(files, &f)
		}
	}
	return files, nil
}

func (c *Client) CleanDuplicates(ctx context.Context, log *zap.Logger) (int, error) {
	log.Info("mongo: starting database duplicate cleanup...")
	var totalDeleted int
	for _, coll := range c.fileCollection.allCollections {
		// 1. Clean up duplicate file_ids
		pipelineFileId := mongo.Pipeline{
			{{Key: "$group", Value: bson.D{
				{Key: "_id", Value: "$file_id"},
				{Key: "ids", Value: bson.D{{Key: "$push", Value: "$_id"}}},
				{Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}},
			}}},
			{{Key: "$match", Value: bson.D{
				{Key: "count", Value: bson.D{{Key: "$gt", Value: 1}}},
			}}},
		}
		cursor, err := coll.Aggregate(ctx, pipelineFileId)
		if err == nil {
			var results []struct {
				FileID string   `bson:"_id"`
				IDs    []string `bson:"ids"`
			}
			if err := cursor.All(ctx, &results); err == nil {
				deletedCount := 0
				for _, res := range results {
					if len(res.IDs) > 1 {
						// Keep the first ID, delete the rest
						toDelete := res.IDs[1:]
						resDel, err := coll.DeleteMany(ctx, bson.M{"_id": bson.M{"$in": toDelete}})
						if err == nil {
							deletedCount += int(resDel.DeletedCount)
						}
					}
				}
				if deletedCount > 0 {
					log.Info("mongo: cleaned up duplicate file_ids", zap.String("collection", coll.Name()), zap.Int("deleted", deletedCount))
					totalDeleted += deletedCount
				}
			}
			cursor.Close(ctx)
		} else {
			log.Warn("mongo: aggregate for duplicate file_ids failed", zap.Error(err), zap.String("collection", coll.Name()))
		}

		// 2. Clean up duplicate file_name + file_size
		pipelineNameSize := mongo.Pipeline{
			{{Key: "$group", Value: bson.D{
				{Key: "_id", Value: bson.D{
					{Key: "name", Value: "$file_name"},
					{Key: "size", Value: "$file_size"},
				}},
				{Key: "ids", Value: bson.D{{Key: "$push", Value: "$_id"}}},
				{Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}},
			}}},
			{{Key: "$match", Value: bson.D{
				{Key: "count", Value: bson.D{{Key: "$gt", Value: 1}}},
			}}},
		}
		cursor, err = coll.Aggregate(ctx, pipelineNameSize)
		if err == nil {
			var results []struct {
				IDs []string `bson:"ids"`
			}
			if err := cursor.All(ctx, &results); err == nil {
				deletedCount := 0
				for _, res := range results {
					if len(res.IDs) > 1 {
						toDelete := res.IDs[1:]
						resDel, err := coll.DeleteMany(ctx, bson.M{"_id": bson.M{"$in": toDelete}})
						if err == nil {
							deletedCount += int(resDel.DeletedCount)
						}
					}
				}
				if deletedCount > 0 {
					log.Info("mongo: cleaned up duplicate name/size files", zap.String("collection", coll.Name()), zap.Int("deleted", deletedCount))
					totalDeleted += deletedCount
				}
			}
			cursor.Close(ctx)
		} else {
			log.Warn("mongo: aggregate for duplicate name/size failed", zap.Error(err), zap.String("collection", coll.Name()))
		}
	}
	log.Info("mongo: database duplicate cleanup completed.", zap.Int("total_deleted", totalDeleted))
	return totalDeleted, nil
}
