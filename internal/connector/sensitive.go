package connector

import (
	"strings"
)

// sensitiveExactTokens contains individual words that indicate sensitive data.
var sensitiveExactTokens = map[string]bool{
	"password":       true,
	"passwd":         true,
	"pwd":            true,
	"passphrase":     true,
	"passcode":       true,
	"secret":         true,
	"token":          true,
	"apikey":         true,
	"salt":           true,
	"ssn":            true,
	"cvv":            true,
	"cvc":            true,
	"pin":            true,
	"privkey":        true,
	"credential":     true,
	"credentials":    true,
	"otp":            true,
	"jwt":            true,
	"bearer":         true,
	"creditcard":     true,
	"cardnumber":     true,
}

// sensitivePhrases contains multi-word normalized substrings that indicate sensitive data.
var sensitivePhrases = []string{
	"password",
	"passwd",
	"pwd",
	"secret",
	"token",
	"api_key",
	"apikey",
	"access_token",
	"refresh_token",
	"auth_token",
	"auth_key",
	"auth_secret",
	"salt",
	"ssn",
	"social_security",
	"credit_card",
	"creditcard",
	"card_number",
	"card_num",
	"cc_num",
	"cvv",
	"cvc",
	"pin",
	"passcode",
	"private_key",
	"privkey",
	"passphrase",
	"credential",
	"otp",
	"national_id",
}

// IsSensitiveColumn returns true if the column name matches known sensitive keywords.
// It avoids false positives on common non-sensitive words such as "author", "authority", etc.
func IsSensitiveColumn(columnName string) bool {
	lower := strings.ToLower(strings.TrimSpace(columnName))
	if lower == "" {
		return false
	}

	// Explicit false-positive guard for author / authors / authority
	if lower == "author" || strings.HasPrefix(lower, "author_") || strings.HasSuffix(lower, "_author") ||
		lower == "authors" || strings.HasPrefix(lower, "authors_") || strings.HasSuffix(lower, "_authors") ||
		lower == "authority" {
		return false
	}

	// Tokenize on underscores, hyphens, dots, and camelCase transitions
	tokens := splitIntoWords(columnName)
	for _, tok := range tokens {
		tokLower := strings.ToLower(tok)
		if sensitiveExactTokens[tokLower] {
			return true
		}
	}

	// Normalize by stripping non-alphanumeric chars
	normalized := strings.ReplaceAll(strings.ReplaceAll(lower, "_", ""), "-", "")
	for _, phrase := range sensitivePhrases {
		phraseNorm := strings.ReplaceAll(phrase, "_", "")
		if strings.Contains(normalized, phraseNorm) {
			return true
		}
	}

	return false
}

// splitIntoWords splits column names by delimiters (_ - .) and camelCase boundaries.
func splitIntoWords(s string) []string {
	var words []string
	var current strings.Builder

	for i, r := range s {
		if r == '_' || r == '-' || r == '.' || r == ' ' {
			if current.Len() > 0 {
				words = append(words, current.String())
				current.Reset()
			}
			continue
		}

		if i > 0 && r >= 'A' && r <= 'Z' {
			prev := rune(s[i-1])
			if (prev >= 'a' && prev <= 'z') || (prev >= '0' && prev <= '9') {
				if current.Len() > 0 {
					words = append(words, current.String())
					current.Reset()
				}
			}
		}

		current.WriteRune(r)
	}

	if current.Len() > 0 {
		words = append(words, current.String())
	}

	return words
}

// FilterSensitiveColumns filters out columns that are either sensitive or in the excluded list.
func FilterSensitiveColumns(columns []ColumnMetadata, excludedNames []string, autoExcludeSensitive bool) []string {
	excludedSet := make(map[string]bool, len(excludedNames))
	for _, name := range excludedNames {
		excludedSet[strings.ToLower(strings.TrimSpace(name))] = true
	}

	var allowed []string
	for _, col := range columns {
		colLower := strings.ToLower(col.Name)
		if excludedSet[colLower] {
			continue
		}
		if autoExcludeSensitive && (col.IsSensitive || IsSensitiveColumn(col.Name)) {
			continue
		}
		allowed = append(allowed, col.Name)
	}
	return allowed
}
