package interactive

import (
	"github.com/pterm/pterm"
)

// Spinner represents an interactive progress spinner.
type Spinner struct {
	spinner *pterm.SpinnerPrinter
	active  bool
}

// StartSpinner starts a spinner with the given message.
func StartSpinner(message string) (*Spinner, error) {
	if !CanPrompt() {
		return &Spinner{active: false}, nil
	}
	sp, err := pterm.DefaultSpinner.Start(message)
	if err != nil {
		return &Spinner{active: false}, nil
	}
	return &Spinner{spinner: sp, active: true}, nil
}

// Update updates the spinner message.
func (s *Spinner) Update(message string) {
	if s != nil && s.active && s.spinner != nil {
		s.spinner.UpdateText(message)
	}
}

// Success stops the spinner with a success message.
func (s *Spinner) Success(message ...interface{}) {
	if s != nil && s.active && s.spinner != nil {
		s.spinner.Success(message...)
		s.active = false
	}
}

// Fail stops the spinner with a failure message.
func (s *Spinner) Fail(message ...interface{}) {
	if s != nil && s.active && s.spinner != nil {
		s.spinner.Fail(message...)
		s.active = false
	}
}

// Stop cleanly terminates the spinner.
func (s *Spinner) Stop() {
	if s != nil && s.active && s.spinner != nil {
		_ = s.spinner.Stop()
		s.active = false
	}
}

// Progressbar represents a progress bar.
type Progressbar struct {
	bar    *pterm.ProgressbarPrinter
	active bool
}

// StartProgressbar starts a progress bar with the given total count and title.
func StartProgressbar(total int, title string) (*Progressbar, error) {
	if !CanPrompt() {
		return &Progressbar{active: false}, nil
	}
	pb, err := pterm.DefaultProgressbar.WithTotal(total).WithTitle(title).Start()
	if err != nil {
		return &Progressbar{active: false}, nil
	}
	return &Progressbar{bar: pb, active: true}, nil
}

// Increment advances the progress bar by 1.
func (p *Progressbar) Increment() {
	if p != nil && p.active && p.bar != nil {
		p.bar.Increment()
	}
}

// Update sets the progress bar current count.
func (p *Progressbar) Update(current int) {
	if p != nil && p.active && p.bar != nil {
		if current > p.bar.Current {
			p.bar.Add(current - p.bar.Current)
		}
	}
}

// Stop cleanly terminates the progress bar.
func (p *Progressbar) Stop() {
	if p != nil && p.active && p.bar != nil {
		_, _ = p.bar.Stop()
		p.active = false
	}
}
