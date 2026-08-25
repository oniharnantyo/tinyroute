package dashboard

import (
	"reflect"
	"testing"
)

func TestGroupModelsByProvider(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []ProviderModelGroup
	}{
		{
			name:     "empty input",
			input:    nil,
			expected: nil,
		},
		{
			name:  "standard provider:model ids",
			input: []string{"anthropic:claude-3-5-sonnet", "openai:gpt-4o", "openai:gpt-4o-mini"},
			expected: []ProviderModelGroup{
				{
					Provider: "anthropic",
					Models: []ModelOption{
						{Value: "anthropic:claude-3-5-sonnet", Label: "claude-3-5-sonnet"},
					},
				},
				{
					Provider: "openai",
					Models: []ModelOption{
						{Value: "openai:gpt-4o", Label: "gpt-4o"},
						{Value: "openai:gpt-4o-mini", Label: "gpt-4o-mini"},
					},
				},
			},
		},
		{
			name:  "combo ids are excluded from provider groups",
			input: []string{"combo:fast", "combo:smart", "openai:gpt-4o"},
			expected: []ProviderModelGroup{
				{
					Provider: "openai",
					Models: []ModelOption{
						{Value: "openai:gpt-4o", Label: "gpt-4o"},
					},
				},
			},
		},
		{
			name:  "unprefixed ids group under defaults",
			input: []string{"custom-model-1", "custom-model-2"},
			expected: []ProviderModelGroup{
				{
					Provider: "defaults",
					Models: []ModelOption{
						{Value: "custom-model-1", Label: "custom-model-1"},
						{Value: "custom-model-2", Label: "custom-model-2"},
					},
				},
			},
		},
		{
			name:  "mixed prefixed unprefixed and combos",
			input: []string{"combo:fast", "defaults-model", "openai:gpt-4o", "combo:smart"},
			expected: []ProviderModelGroup{
				{
					Provider: "defaults",
					Models: []ModelOption{
						{Value: "defaults-model", Label: "defaults-model"},
					},
				},
				{
					Provider: "openai",
					Models: []ModelOption{
						{Value: "openai:gpt-4o", Label: "gpt-4o"},
					},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := groupModelsByProvider(tc.input)
			if !reflect.DeepEqual(got, tc.expected) {
				t.Errorf("groupModelsByProvider(%v) =\n  %+v\nwant:\n  %+v", tc.input, got, tc.expected)
			}
		})
	}
}

func TestSplitComboOptions(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []ModelOption
	}{
		{
			name:     "empty input",
			input:    nil,
			expected: nil,
		},
		{
			name:     "no combos in input",
			input:    []string{"openai:gpt-4o", "anthropic:claude-3-5"},
			expected: nil,
		},
		{
			name:  "combos extracted with stripped prefix labels",
			input: []string{"combo:fast", "openai:gpt-4o", "combo:smart"},
			expected: []ModelOption{
				{Value: "combo:fast", Label: "fast"},
				{Value: "combo:smart", Label: "smart"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := splitComboOptions(tc.input)
			if !reflect.DeepEqual(got, tc.expected) {
				t.Errorf("splitComboOptions(%v) =\n  %+v\nwant:\n  %+v", tc.input, got, tc.expected)
			}
		})
	}
}
