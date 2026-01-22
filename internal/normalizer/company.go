package normalizer

import (
	"database/sql"
	"regexp"
	"strings"
)

// CompanyNormalizer handles company name normalization and grouping
type CompanyNormalizer struct {
	mappings map[string]string
	patterns []CompanyPattern
}

// CompanyPattern represents a regex pattern for company name matching
type CompanyPattern struct {
	Pattern *regexp.Regexp
	Company string
}

// NewCompanyNormalizer creates a normalizer with predefined mappings
func NewCompanyNormalizer() *CompanyNormalizer {
	normalizer := &CompanyNormalizer{
		mappings: make(map[string]string),
	}

	// Define exact name mappings
	normalizer.mappings = map[string]string{
		// Intel variations
		"Intel Corporation":                         "Intel",
		"Intel":                                     "Intel",
		"Intel Corporation (SC-12)":                 "Intel",
		"Intel Corporation (Robert Noyce)":          "Intel",
		"Intel Corporation (SC-9)":                  "Intel",
		"Intel Corporation (SC-2)":                  "Intel",
		"Intel Corporation (SC-1)":                  "Intel",
		"Intel Corporation (SC-11)":                 "Intel",
		"Intel Corporation - Robert Noyce Building": "Intel",
		"Intel Corporation - Robert Noyce":          "Intel",
		"Intel Corporation - SC-12":                 "Intel",
		"Intel Corporation - SC-9":                  "Intel",
		"Intel Corporation - SC-1 3065 Bowers":      "Intel",
		"Intel Corporation - SC-2":                  "Intel",
		"Intel Corporation - SC-11":                 "Intel",
		"Intel Corporation (Robert Noyce Building)": "Intel",
		"Honeywell Inteligrated LLC":                "Honeywell",
		"InteLogix":                                 "InteLogix",

		// Google/Alphabet variations
		"Google":        "Google",
		"Google LLC":    "Google",
		"Alphabet Inc.": "Google",
		"Alphabet":      "Google",

		// Microsoft variations
		"Microsoft":             "Microsoft",
		"Microsoft Corporation": "Microsoft",

		// Amazon variations
		"Amazon":              "Amazon",
		"Amazon.com":          "Amazon",
		"Amazon.com, Inc.":    "Amazon",
		"Amazon Web Services": "Amazon",

		// Meta/Facebook variations
		"Meta":                 "Meta",
		"Meta Platforms":       "Meta",
		"Meta Platforms, Inc.": "Meta",
		"Facebook":             "Meta",
		"Facebook, Inc.":       "Meta",

		// Apple variations
		"Apple":          "Apple",
		"Apple Inc.":     "Apple",
		"Apple Computer": "Apple",

		// Tesla variations
		"Tesla":       "Tesla",
		"Tesla, Inc.": "Tesla",

		// Netflix variations
		"Netflix":       "Netflix",
		"Netflix, Inc.": "Netflix",

		// Twitter/X variations
		"Twitter": "X Corp",
		"X Corp":  "X Corp",
		"X":       "X Corp",

		// Aerospace and defense
		"Boeing":                       "Boeing",
		"The Boeing Company":           "Boeing",
		"Northrop Grumman":             "Northrop Grumman",
		"Northrop Grumman Corporation": "Northrop Grumman",

		// Airlines
		"United Airlines":       "United Airlines",
		"United Airlines, Inc.": "United Airlines",
		"united airlines":       "United Airlines",

		// Other major companies
		"Walmart":            "Walmart",
		"Walmart Inc.":       "Walmart",
		"Target":             "Target",
		"Target Corporation": "Target",
		"Costco":             "Costco",
		"Costco Wholesale":   "Costco",
		"Home Depot":         "Home Depot",
		"The Home Depot":     "Home Depot",
		"Home Depot, Inc.":   "Home Depot",
		"Lowes":              "Lowe's",
		"Lowe's":             "Lowe's",
		"Lowe's Companies":   "Lowe's",
		"Kroger":             "Kroger",
		"The Kroger Co.":     "Kroger",

		// Financial companies
		"JPMorgan Chase":  "JPMorgan Chase",
		"JPMorgan":        "JPMorgan Chase",
		"J.P. Morgan":     "JPMorgan Chase",
		"Goldman Sachs":   "Goldman Sachs",
		"Goldman":         "Goldman Sachs",
		"Bank of America": "Bank of America",
		"BOA":             "Bank of America",
		"Wells Fargo":     "Wells Fargo",
		"Morgan Stanley":  "Morgan Stanley",

		// Healthcare companies
		"Johnson & Johnson":     "Johnson & Johnson",
		"J&J":                   "Johnson & Johnson",
		"Pfizer":                "Pfizer",
		"Pfizer Inc.":           "Pfizer",
		"Merck":                 "Merck",
		"Merck & Co.":           "Merck",
		"AbbVie":                "AbbVie",
		"AbbVie Inc.":           "AbbVie",
		"Bristol Myers Squibb":  "Bristol Myers Squibb",
		"Bristol-Myers Squibb":  "Bristol Myers Squibb",
		"Eli Lilly":             "Eli Lilly",
		"Eli Lilly and Company": "Eli Lilly",
	}

	// Define regex patterns for more flexible matching
	patterns := []struct {
		pattern string
		company string
	}{
		{`(?i)^intel\b.*`, "Intel"},
		{`(?i)^google\b.*`, "Google"},
		{`(?i)^microsoft\b.*`, "Microsoft"},
		{`(?i)^amazon\b.*`, "Amazon"},
		{`(?i)^meta\b.*`, "Meta"},
		{`(?i)^facebook\b.*`, "Meta"},
		{`(?i)^apple\b.*`, "Apple"},
		{`(?i)^tesla\b.*`, "Tesla"},
		{`(?i)^netflix\b.*`, "Netflix"},
		{`(?i)^twitter\b.*`, "X Corp"},
		{`(?i)^walmart\b.*`, "Walmart"},
		{`(?i)^target\b.*`, "Target"},
		{`(?i)^costco\b.*`, "Costco"},
		{`(?i)^home depot\b.*`, "Home Depot"},
		{`(?i)^lowe'?s\b.*`, "Lowe's"},
		{`(?i)^kroger\b.*`, "Kroger"},
		{`(?i)^jpmorgan\b.*`, "JPMorgan Chase"},
		{`(?i)^goldman\b.*`, "Goldman Sachs"},
		{`(?i)^bank of america\b.*`, "Bank of America"},
		{`(?i)^wells fargo\b.*`, "Wells Fargo"},
		{`(?i)^morgan stanley\b.*`, "Morgan Stanley"},
		{`(?i)^boeing\b.*`, "Boeing"},
		{`(?i)^northrop\b.*`, "Northrop Grumman"},
		{`(?i)^united airlines?\b.*`, "United Airlines"},
		{`(?i)^johnson.?&.?johnson\b.*`, "Johnson & Johnson"},
		{`(?i)^pfizer\b.*`, "Pfizer"},
		{`(?i)^merck\b.*`, "Merck"},
		{`(?i)^abbvie\b.*`, "AbbVie"},
		{`(?i)^bristol.?myers\b.*`, "Bristol Myers Squibb"},
		{`(?i)^eli lilly\b.*`, "Eli Lilly"},
	}

	for _, p := range patterns {
		regex, err := regexp.Compile(p.pattern)
		if err != nil {
			continue // Skip invalid patterns
		}
		normalizer.patterns = append(normalizer.patterns, CompanyPattern{
			Pattern: regex,
			Company: p.company,
		})
	}

	return normalizer
}

// NormalizeCompany normalizes a company name to its canonical form
func (n *CompanyNormalizer) NormalizeCompany(companyName string) string {
	if companyName == "" {
		return companyName
	}

	name := strings.TrimSpace(companyName)

	// First check exact mappings
	if canonical, exists := n.mappings[name]; exists {
		return canonical
	}

	// Then check regex patterns
	for _, pattern := range n.patterns {
		if pattern.Pattern.MatchString(name) {
			return pattern.Company
		}
	}

	// Return original name if no match found
	return name
}

// GetTopNormalizedCompanies returns the top companies by employee impact with normalization
func (n *CompanyNormalizer) GetTopNormalizedCompanies(db *sql.DB, limit int) ([]CompanyStats, error) {
	// This will be implemented in the service layer
	return nil, nil
}

// CompanyStats represents aggregated stats for a normalized company
type CompanyStats struct {
	Company   string `json:"company"`
	Employees int    `json:"employees"`
	Layoffs   int    `json:"layoffs"`
}
