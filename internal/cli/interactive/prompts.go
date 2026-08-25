package interactive

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/pterm/pterm"
	"golang.org/x/term"
)

var (
	canPromptOverride *bool
	inputFn           func(message, defaultVal string, validator func(string) error) (string, error)
	selectFn          func(message string, options []string) (string, error)
	multiSelectFn     func(message string, options []string) ([]string, error)
	confirmFn         func(message string, defaultVal bool) (bool, error)
)

// SetCanPromptOverride forces CanPrompt to return the specified boolean value for testing.
// Passing nil resets the override.
func SetCanPromptOverride(override *bool) {
	canPromptOverride = override
}

// SetInputOverride overrides Input behavior for testing.
func SetInputOverride(fn func(message, defaultVal string, validator func(string) error) (string, error)) {
	inputFn = fn
}

// SetSelectOverride overrides Select behavior for testing.
func SetSelectOverride(fn func(message string, options []string) (string, error)) {
	selectFn = fn
}

// SetMultiSelectOverride overrides MultiSelect behavior for testing.
func SetMultiSelectOverride(fn func(message string, options []string) ([]string, error)) {
	multiSelectFn = fn
}

// SetConfirmOverride overrides Confirm behavior for testing.
func SetConfirmOverride(fn func(message string, defaultVal bool) (bool, error)) {
	confirmFn = fn
}

// CanPrompt checks if stdin and stdout are interactive terminals.
func CanPrompt() bool {
	if canPromptOverride != nil {
		return *canPromptOverride
	}
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

// Confirm asks the user for a yes/no confirmation.
// If CanPrompt() is false, it automatically returns defaultVal.
func Confirm(message string, defaultVal bool) (bool, error) {
	if confirmFn != nil {
		return confirmFn(message, defaultVal)
	}
	if !CanPrompt() {
		return defaultVal, nil
	}
	prompt := pterm.DefaultInteractiveConfirm.WithDefaultValue(defaultVal)
	return prompt.Show(message)
}

// Password prompts the user for masked password/credential entry.
// If CanPrompt() is false, it falls back to reading a line from os.Stdin.
func Password(message string) (string, error) {
	if !CanPrompt() {
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil && len(line) == 0 {
			return "", err
		}
		return strings.TrimRight(line, "\r\n"), nil
	}
	prompt := pterm.DefaultInteractiveTextInput.WithMask("*")
	return prompt.Show(message)
}

// Input prompts the user for text input, with optional default value and validator.
// If CanPrompt() is false, it returns defaultVal if provided (validated if validator present), or reads from os.Stdin.
func Input(message string, defaultVal string, validator func(string) error) (string, error) {
	if inputFn != nil {
		return inputFn(message, defaultVal, validator)
	}
	if !CanPrompt() {
		if defaultVal != "" {
			if validator != nil {
				if err := validator(defaultVal); err != nil {
					return "", err
				}
			}
			return defaultVal, nil
		}
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil && len(line) == 0 {
			return "", err
		}
		res := strings.TrimRight(line, "\r\n")
		if validator != nil {
			if err := validator(res); err != nil {
				return "", err
			}
		}
		return res, nil
	}
	prompt := pterm.DefaultInteractiveTextInput.WithDefaultValue(defaultVal)
	for {
		val, err := prompt.Show(message)
		if err != nil {
			return "", err
		}
		if validator != nil {
			if err := validator(val); err != nil {
				pterm.Error.Println("Invalid input: " + err.Error())
				continue
			}
		}
		return val, nil
	}
}

// Select prompts the user to choose an option from a list.
// If CanPrompt() is false, it returns the first option if available.
func Select(message string, options []string) (string, error) {
	if selectFn != nil {
		return selectFn(message, options)
	}
	if !CanPrompt() {
		if len(options) > 0 {
			return options[0], nil
		}
		return "", fmt.Errorf("no options available")
	}
	prompt := pterm.DefaultInteractiveSelect.WithOptions(options)
	return prompt.Show(message)
}

// MultiSelect prompts the user to choose multiple options from a list.
// If CanPrompt() is false, it returns all options.
func MultiSelect(message string, options []string) ([]string, error) {
	if multiSelectFn != nil {
		return multiSelectFn(message, options)
	}
	if !CanPrompt() {
		return options, nil
	}
	if len(options) == 0 {
		return nil, fmt.Errorf("no options available")
	}
	prompt := pterm.DefaultInteractiveMultiselect.WithOptions(options)
	return prompt.Show(message)
}
