package agent

import (
	"os"
	"os/exec"
	"runtime"
)

type devinAdapter struct{}

func init() {
	Register(&devinAdapter{})
}

func (d *devinAdapter) ID() string       { return "devin" }
func (d *devinAdapter) Name() string     { return "Devin CLI" }
func (d *devinAdapter) Dialect() string  { return "openai" }
func (d *devinAdapter) NeedsModel() bool { return false }

func (d *devinAdapter) ModelSlots() []ModelSlot {
	return nil
}

func (d *devinAdapter) candidatePaths() []string {
	if runtime.GOOS == "windows" {
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData == "" {
			localAppData = expandHome("~/AppData/Local")
		}
		return []string{
			localAppData + `\devin\cli\bin\devin.exe`,
			expandHome(`~/.local/bin/devin.exe`),
			expandHome(`~/scoop/shims/devin.exe`),
			localAppData + `\Programs\devin\devin.exe`,
		}
	}
	return []string{
		expandHome("~/.local/share/devin/bin/devin"),
		expandHome("~/.devin/bin/devin"),
		expandHome("~/.local/bin/devin"),
		"/opt/homebrew/bin/devin",
		"/usr/local/bin/devin",
		"/usr/bin/devin",
	}
}

func (d *devinAdapter) Detect() (Status, error) {
	cmdName := "which"
	if runtime.GOOS == "windows" {
		cmdName = "where"
	}
	installed := false
	detectedPath := ""

	if err := exec.Command(cmdName, "devin").Run(); err == nil {
		installed = true
		detectedPath = "PATH"
	} else {
		for _, p := range d.candidatePaths() {
			if _, err := os.Stat(p); err == nil {
				installed = true
				detectedPath = p
				break
			}
		}
	}

	return Status{
		Installed:          installed,
		PointedAtTinyRoute: false,
		ConfigPath:         detectedPath,
	}, nil
}

func (d *devinAdapter) Apply(input ApplyInput) (Result, error) {
	st, _ := d.Detect()
	return Result{
		Files: []string{st.ConfigPath},
	}, nil
}

func (d *devinAdapter) Reset() error {
	return nil
}
