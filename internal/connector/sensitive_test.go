package connector

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsSensitiveColumn(t *testing.T) {
	sensitive := []string{
		"password",
		"PASSWORD",
		"password_hash",
		"user_password",
		"pwd",
		"user_pwd",
		"api_key",
		"apiKey",
		"secret_key",
		"auth_token",
		"access_token",
		"credit_card",
		"card_number",
		"card_num",
		"cc_num",
		"ssn",
		"private_key",
		"otp",
		"passcode",
		"national_id",
	}

	for _, name := range sensitive {
		assert.True(t, IsSensitiveColumn(name), "expected %s to be sensitive", name)
	}

	nonSensitive := []string{
		"id",
		"name",
		"email",
		"username",
		"created_at",
		"title",
		"description",
		"price",
		"status",
		"quantity",
		"author",
		"author_id",
		"author_name",
		"authority",
	}

	for _, name := range nonSensitive {
		assert.False(t, IsSensitiveColumn(name), "expected %s not to be sensitive", name)
	}
}

func TestFilterSensitiveColumns(t *testing.T) {
	cols := []ColumnMetadata{
		{Name: "id", IsSensitive: false},
		{Name: "username", IsSensitive: false},
		{Name: "password_hash", IsSensitive: true},
		{Name: "email", IsSensitive: false},
		{Name: "secret_token", IsSensitive: true},
		{Name: "internal_notes", IsSensitive: false},
	}

	filtered := FilterSensitiveColumns(cols, []string{"internal_notes"}, true)
	assert.Equal(t, []string{"id", "username", "email"}, filtered)

	// With sensitive allowed
	allowedAll := FilterSensitiveColumns(cols, []string{"internal_notes"}, false)
	assert.Equal(t, []string{"id", "username", "password_hash", "email", "secret_token"}, allowedAll)
}
