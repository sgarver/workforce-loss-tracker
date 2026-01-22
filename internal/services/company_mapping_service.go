package services

import (
	"log"
	"sort"
	"strings"
	"unicode"

	"layoff-tracker/internal/database"
)

// CompanyMappingService handles dynamic company name normalization
type CompanyMappingService struct {
	db *database.DB
}

// NewCompanyMappingService creates a new mapping service
func NewCompanyMappingService(db *database.DB) *CompanyMappingService {
	return &CompanyMappingService{db: db}
}

// NormalizeCompany normalizes a company name using database mappings and fuzzy matching
func (s *CompanyMappingService) NormalizeCompany(companyName string) (string, error) {
	if companyName == "" {
		return companyName, nil
	}

	name := strings.TrimSpace(companyName)

	// Apply basic normalization rules first
	normalized := s.applyBasicNormalization(name)
	if normalized != name {
		return normalized, nil
	}

	// First check for exact mapping in database
	var canonicalName string
	err := s.db.QueryRow("SELECT canonical_name FROM company_mappings WHERE original_name = ? AND confidence_score >= 80", name).Scan(&canonicalName)
	if err == nil {
		return canonicalName, nil
	}

	// Try fuzzy matching based on similarity
	canonicalName, err = s.findBestFuzzyMatch(name)
	if err == nil && canonicalName != "" {
		// Auto-create mapping for future use
		s.createMapping(name, canonicalName, "auto", 75)
		return canonicalName, nil
	}

	// No match found, return original name
	return name, nil
}

// findBestFuzzyMatch finds the best fuzzy match for a company name
func (s *CompanyMappingService) findBestFuzzyMatch(companyName string) (string, error) {
	name := strings.ToLower(companyName)

	// Get all existing canonical names
	rows, err := s.db.Query("SELECT DISTINCT canonical_name FROM company_mappings WHERE confidence_score >= 90")
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var candidates []string
	for rows.Next() {
		var canonical string
		if err := rows.Scan(&canonical); err != nil {
			continue
		}
		candidates = append(candidates, canonical)
	}

	// Score each candidate
	type match struct {
		canonical string
		score     int
	}

	var matches []match
	for _, candidate := range candidates {
		score := s.calculateSimilarityScore(name, strings.ToLower(candidate))
		if score >= 60 { // Minimum threshold for fuzzy matching
			matches = append(matches, match{canonical: candidate, score: score})
		}
	}

	// Return best match if any
	if len(matches) > 0 {
		sort.Slice(matches, func(i, j int) bool {
			return matches[i].score > matches[j].score
		})
		return matches[0].canonical, nil
	}

	return "", nil
}

// calculateSimilarityScore calculates how similar two company names are
func (s *CompanyMappingService) calculateSimilarityScore(name1, name2 string) int {
	// Clean both names
	clean1 := s.cleanCompanyName(name1)
	clean2 := s.cleanCompanyName(name2)

	// Exact match after cleaning
	if clean1 == clean2 {
		return 100
	}

	// One contains the other
	if strings.Contains(clean1, clean2) || strings.Contains(clean2, clean1) {
		shorter := len(clean1)
		if len(clean2) < shorter {
			shorter = len(clean2)
		}
		longer := len(clean1)
		if len(clean2) > longer {
			longer = len(clean2)
		}
		return 80 + (shorter * 20 / longer) // Bonus based on length similarity
	}

	// Word-based similarity
	words1 := s.tokenizeCompanyName(clean1)
	words2 := s.tokenizeCompanyName(clean2)

	commonWords := 0
	totalWords := len(words1) + len(words2)

	for _, w1 := range words1 {
		for _, w2 := range words2 {
			if s.wordsSimilar(w1, w2) {
				commonWords++
				break
			}
		}
	}

	if totalWords == 0 {
		return 0
	}

	similarity := (commonWords * 2 * 100) / totalWords
	return similarity
}

// cleanCompanyName normalizes spacing and removes punctuation for comparison
func (s *CompanyMappingService) cleanCompanyName(name string) string {
	name = strings.ToLower(name)

	// Remove punctuation but keep suffixes for matching
	name = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
			return r
		}
		return -1
	}, name)

	return strings.Join(strings.Fields(name), " ")
}

// cleanCompanyNameForDisplay normalizes company names for display by removing punctuation and extra spaces
func (s *CompanyMappingService) cleanCompanyNameForDisplay(name string) string {
	// Remove punctuation and normalize spacing
	name = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
			return r
		}
		return -1
	}, name)

	return strings.Join(strings.Fields(name), " ")
}

// tokenizeCompanyName splits name into meaningful words
func (s *CompanyMappingService) tokenizeCompanyName(name string) []string {
	words := strings.Fields(name)

	// Filter out common stop words
	stopWords := map[string]bool{
		"the": true, "and": true, "or": true, "of": true, "to": true, "a": true, "an": true,
		"for": true, "by": true, "with": true, "as": true, "at": true, "from": true,
	}

	var filtered []string
	for _, word := range words {
		if len(word) > 2 && !stopWords[word] {
			filtered = append(filtered, word)
		}
	}

	return filtered
}

// wordsSimilar checks if two words are similar (exact match, substring, or edit distance)
func (s *CompanyMappingService) wordsSimilar(w1, w2 string) bool {
	// Exact match
	if w1 == w2 {
		return true
	}

	// One contains the other
	if strings.Contains(w1, w2) || strings.Contains(w2, w1) {
		return true
	}

	// Simple edit distance (only for short words)
	if len(w1) <= 6 && len(w2) <= 6 {
		return s.levenshteinDistance(w1, w2) <= 1
	}

	return false
}

// levenshteinDistance calculates edit distance between two strings
func (s *CompanyMappingService) levenshteinDistance(s1, s2 string) int {
	if len(s1) == 0 {
		return len(s2)
	}
	if len(s2) == 0 {
		return len(s1)
	}

	matrix := make([][]int, len(s1)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(s2)+1)
		matrix[i][0] = i
	}
	for j := range matrix[0] {
		matrix[0][j] = j
	}

	for i := 1; i <= len(s1); i++ {
		for j := 1; j <= len(s2); j++ {
			cost := 0
			if s1[i-1] != s2[j-1] {
				cost = 1
			}

			matrix[i][j] = min(
				min(matrix[i-1][j]+1, matrix[i][j-1]+1), // deletion, insertion
				matrix[i-1][j-1]+cost,                   // substitution
			)
		}
	}

	return matrix[len(s1)][len(s2)]
}

// createMapping creates a new company mapping
func (s *CompanyMappingService) createMapping(originalName, canonicalName, mappingType string, confidence int) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO company_mappings
		(original_name, canonical_name, mapping_type, confidence_score, updated_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		originalName, canonicalName, mappingType, confidence)
	return err
}

// GetCanonicalName gets the canonical name for a company
func (s *CompanyMappingService) GetCanonicalName(originalName string) (string, error) {
	canonical, err := s.NormalizeCompany(originalName)
	return canonical, err
}

// UpdateCompanyCanonicalNames updates all companies with their canonical names
func (s *CompanyMappingService) UpdateCompanyCanonicalNames() error {
	rows, err := s.db.Query("SELECT id, name FROM companies WHERE canonical_name IS NULL OR canonical_name = ''")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			continue
		}

		canonicalName, err := s.NormalizeCompany(name)
		if err != nil {
			continue
		}

		// Update the company record
		_, err = s.db.Exec(`
			UPDATE companies
			SET canonical_name = ?, updated_at = CURRENT_TIMESTAMP
			WHERE id = ?`,
			canonicalName, id)
		if err != nil {
			return err
		}
	}

	return nil
}

// UpdateAllCompanyCanonicalNames forces update of canonical names for all companies
func (s *CompanyMappingService) UpdateAllCompanyCanonicalNames() error {
	rows, err := s.db.Query("SELECT id, name FROM companies")
	if err != nil {
		return err
	}
	defer rows.Close()

	updated := 0
	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			continue
		}

		canonicalName, err := s.NormalizeCompany(name)
		if err != nil {
			continue
		}

		// Update the company record
		_, err = s.db.Exec(`
			UPDATE companies
			SET canonical_name = ?, updated_at = CURRENT_TIMESTAMP
			WHERE id = ?`,
			canonicalName, id)
		if err != nil {
			return err
		}
		updated++
	}

	log.Printf("Updated canonical names for %d companies", updated)
	return nil
}

// applyBasicNormalization applies basic normalization rules for well-known companies
func (s *CompanyMappingService) applyBasicNormalization(companyName string) string {
	name := strings.TrimSpace(strings.ToLower(companyName))

	// Clean punctuation and normalize spacing first
	name = s.cleanCompanyNameForDisplay(name)

	// A&P variations - comprehensive matching for all A&P company names
	if strings.Contains(name, "a&p") ||
		(strings.Contains(name, "atlantic") && strings.Contains(name, "pacific")) ||
		strings.Contains(name, "great atlantic and pacific tea") {
		return "A&P"
	}

	// Boeing variations
	if strings.Contains(name, "boeing") {
		return "Boeing"
	}

	// Intel variations
	if strings.Contains(name, "intel") && !strings.Contains(name, "intelli") {
		return "Intel"
	}

	// Wells Fargo variations
	if strings.Contains(name, "wells fargo") {
		return "Wells Fargo"
	}

	// Bank of America variations
	if strings.Contains(name, "bank of america") {
		return "Bank of America"
	}

	// Walmart variations
	if strings.Contains(name, "walmart") {
		return "Walmart"
	}

	// Microsoft variations
	if strings.Contains(name, "microsoft") {
		return "Microsoft"
	}

	// Google variations
	if strings.Contains(name, "google") || strings.Contains(name, "alphabet") {
		return "Google"
	}

	// Amazon variations
	if strings.Contains(name, "amazon") {
		return "Amazon"
	}

	// Meta variations
	if strings.Contains(name, "meta") || strings.Contains(name, "facebook") {
		return "Meta"
	}

	// Apple variations
	if strings.Contains(name, "apple") && !strings.Contains(name, "pineapple") {
		return "Apple"
	}

	// Tesla variations - remove "Motors" as it's a product line
	if strings.Contains(name, "tesla") {
		result := strings.ReplaceAll(name, "motors", "")
		result = strings.ReplaceAll(result, "inc", "")
		result = strings.TrimSpace(result)
		if len(result) > 0 {
			return strings.ToUpper(result[:1]) + result[1:]
		}
		return "Tesla"
	}

	// For other companies, apply punctuation cleaning and basic suffix removal
	result := s.cleanCompanyNameForDisplay(companyName) // Clean the original name
	if s.shouldRemoveSuffix(result) {
		result = s.removeCommonSuffixesConservative(result)
	}
	// Capitalize first letter
	if len(result) > 0 {
		result = strings.ToUpper(result[:1]) + result[1:]
	}

	return result
}

// shouldRemoveSuffix determines if it's safe to remove common suffixes
func (s *CompanyMappingService) shouldRemoveSuffix(name string) bool {
	lowerName := strings.ToLower(name)

	// Only remove suffixes from very long names or names that are clearly corporations
	// Avoid removing from short names, brand names, or names that would become meaningless
	if len(name) < 15 {
		return false // Don't modify short names
	}

	// Don't modify well-known brand names
	brands := []string{"apple", "google", "amazon", "microsoft", "meta", "facebook", "twitter", "netflix"}
	for _, brand := range brands {
		if strings.Contains(lowerName, brand) {
			return false
		}
	}

	return true
}

// removeCommonSuffixesConservative removes common corporate suffixes only when safe to do so
func (s *CompanyMappingService) removeCommonSuffixesConservative(name string) string {
	if !s.shouldRemoveSuffix(name) {
		return name
	}

	// Conservative list - only remove the most obvious corporate suffixes
	suffixes := []string{
		" corporation", " incorporated", " inc.", " inc", " corp.", " corp", " llc", " llp", " ltd.", " ltd",
	}

	result := strings.ToLower(name)

	// Remove suffixes from the end
	for _, suffix := range suffixes {
		if strings.HasSuffix(result, suffix) {
			result = strings.TrimSuffix(result, suffix)
			break // Only remove one suffix
		}
	}

	// If result is too short or meaningless, don't modify
	if len(result) < 3 || result == "the" || result == "and" {
		return name
	}

	// Clean up extra spaces and capitalize first letter
	result = strings.TrimSpace(result)
	if len(result) > 0 {
		result = strings.ToUpper(result[:1]) + result[1:]
	}

	return result
}

// removeCommonSuffixes removes common corporate suffixes to create cleaner company names
func (s *CompanyMappingService) removeCommonSuffixes(name string) string {
	// Common suffixes to remove (in order of specificity)
	suffixes := []string{
		" corporation", " incorporated", " inc.", " inc", " corp.", " corp", " co.", " co", " llc", " llp", " ltd.", " ltd",
		" company", " companies", " group", " holding", " holdings", " international", " global", " systems", " solutions",
		" technologies", " technology", " services", " service", " associates", " partners", " partner",
	}

	result := name

	// Remove suffixes from the end
	for _, suffix := range suffixes {
		if strings.HasSuffix(result, suffix) {
			result = strings.TrimSuffix(result, suffix)
			break // Only remove one suffix
		}
	}

	// Clean up extra spaces and return
	return strings.TrimSpace(result)
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
