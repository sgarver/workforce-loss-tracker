package classifier

import (
	"regexp"
	"strings"
)

// IndustryRule represents a classification rule
type IndustryRule struct {
	Pattern  *regexp.Regexp
	Industry string
	Priority int // Lower number = higher priority
}

// IndustryClassifier handles company industry classification
type IndustryClassifier struct {
	rules []IndustryRule
}

// NewIndustryClassifier creates a new classifier with predefined rules
func NewIndustryClassifier() *IndustryClassifier {
	classifier := &IndustryClassifier{}

	// Define rules with regex patterns
	rules := []struct {
		pattern  string
		industry string
		priority int
	}{
		// High priority - very specific
		{`(?i)\b(apple|google|microsoft|amazon|meta|facebook|netflix|tesla|twitter|x corp)\b`, "Technology", 1},
		{`(?i)\b(jpmorgan|goldman|bank of america|wells fargo|morgan stanley)\b`, "Financial Services", 1},
		{`(?i)\b(johnson & johnson|pfizer|merck|abbvie|bristol myers|eli lilly)\b`, "Healthcare", 1},
		{`(?i)\b(walmart|target|costco|home depot|lowes|kroger)\b`, "Consumer Defensive", 1},
		{`(?i)\b(uber|lyft|airbnb|doordash|instacart)\b`, "Consumer Cyclical", 1},

		// Medium priority - industry keywords
		{`(?i)(software|tech|digital|cloud|ai|cyber|data|analytics|automation)`, "Technology", 2},
		{`(?i)(bank|finance|financial|credit|loan|investment|insurance)`, "Financial Services", 2},
		{`(?i)(health|medical|pharma|biotech|clinical|therapy|hospital)`, "Healthcare", 2},
		{`(?i)(retail|store|e-?commerce|shopping|consumer|restaurant|food)`, "Consumer Cyclical", 2},
		{`(?i)(grocery|beverage|supermarket|wholesale|distribution)`, "Consumer Defensive", 2},
		{`(?i)(manufacturing|industrial|factory|production|engineering)`, "Industrials", 2},
		{`(?i)(energy|oil|gas|renewable|solar|wind|mining)`, "Energy", 2},
		{`(?i)(real estate|property|construction|building|housing)`, "Real Estate", 2},
		{`(?i)(communication|telecom|media|entertainment|cable|satellite)`, "Communication Services", 2},
		{`(?i)(material|chemical|metal|mining|commodity)`, "Basic Materials", 2},
		{`(?i)(utility|power|electric|water|gas utility)`, "Utilities", 2},

		// Lower priority - generic terms
		{`(?i)(corp|corporation|inc|llc|company|ltd|limited)`, "Unknown", 3}, // Low priority fallback
	}

	for _, rule := range rules {
		regex, err := regexp.Compile(rule.pattern)
		if err != nil {
			// Skip invalid regex
			continue
		}
		classifier.rules = append(classifier.rules, IndustryRule{
			Pattern:  regex,
			Industry: rule.industry,
			Priority: rule.priority,
		})
	}

	// Sort rules by priority (lower number first)
	// Note: In Go, we can sort later if needed, but for now append in priority order

	return classifier
}

// ClassifyIndustry classifies a company based on its name
func (c *IndustryClassifier) ClassifyIndustry(companyName string) (industry string, confidence int) {
	if companyName == "" {
		return "Unknown", 0
	}

	// Clean the company name
	name := strings.ToLower(strings.TrimSpace(companyName))

	// Track best match
	bestMatch := "Unknown"
	bestPriority := 999
	matchCount := 0

	// Check each rule
	for _, rule := range c.rules {
		if rule.Pattern.MatchString(name) {
			matchCount++
			if rule.Priority < bestPriority {
				bestMatch = rule.Industry
				bestPriority = rule.Priority
			}
		}
	}

	// Calculate confidence based on priority and match count
	if bestMatch == "Unknown" {
		confidence = 0
	} else {
		// Higher confidence for lower priority numbers and multiple matches
		confidence = max(min(100-(bestPriority*20)+(matchCount*5), 100), 10)
	}

	return bestMatch, confidence
}

// GetIndustryStats returns statistics about the classifier
func (c *IndustryClassifier) GetIndustryStats() map[string]int {
	stats := make(map[string]int)
	for _, rule := range c.rules {
		stats[rule.Industry]++
	}
	return stats
}
