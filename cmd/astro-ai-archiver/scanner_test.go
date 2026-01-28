package main

import (
	"testing"
)

func TestNormalizeTarget(t *testing.T) {
	// Create a minimal scanner instance for testing
	s := &Scanner{}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "M with space",
			input:    "M 31",
			expected: "M31",
		},
		{
			name:     "NGC with space",
			input:    "NGC 7822",
			expected: "NGC7822",
		},
		{
			name:     "IC with space",
			input:    "IC 434",
			expected: "IC434",
		},
		{
			name:     "M without space",
			input:    "M31",
			expected: "M31",
		},
		{
			name:     "NGC with multiple spaces",
			input:    "NGC  7822",
			expected: "NGC7822",
		},
		{
			name:     "M with additional text",
			input:    "M 31 Andromeda Galaxy",
			expected: "M31_Andromeda_Galaxy",
		},
		{
			name:     "NGC with additional text",
			input:    "NGC 7822 Region",
			expected: "NGC7822_Region",
		},
		{
			name:     "Already normalized with additional text",
			input:    "M31 Andromeda",
			expected: "M31_Andromeda",
		},
		{
			name:     "Lowercase prefix",
			input:    "ngc 6888",
			expected: "NGC6888",
		},
		{
			name:     "Mixed case with text",
			input:    "Ngc 1234 Test Object",
			expected: "NGC1234_Test_Object",
		},
		{
			name:     "Multiple words after catalog number",
			input:    "M 51 Whirlpool Galaxy",
			expected: "M51_Whirlpool_Galaxy",
		},
		{
			name:     "No catalog prefix (plain name)",
			input:    "Andromeda Galaxy",
			expected: "Andromeda_Galaxy",
		},
		{
			name:     "Single word",
			input:    "Andromeda",
			expected: "Andromeda",
		},
		{
			name:     "SH2 with space",
			input:    "SH2 159",
			expected: "SH2-159",
		},
		{
			name:     "sh2 lowercase with space",
			input:    "sh2 159",
			expected: "SH2-159",
		},
		{
			name:     "SH2 with text",
			input:    "SH2 159 Flying Bat Nebula",
			expected: "SH2-159_Flying_Bat_Nebula",
		},
		{
			name:     "IC lowercase",
			input:    "ic 434",
			expected: "IC434",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.normalizeTarget(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeTarget(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}
