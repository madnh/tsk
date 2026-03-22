package output

import (
	"fmt"
	"strings"
)

// ANSI escape codes
const (
	Reset     = "\033[0m"
	Bold      = "\033[1m"
	Dim       = "\033[2m"
	Underline = "\033[4m"
)

// Foreground colors
const (
	Red     = "\033[31m"
	Green   = "\033[32m"
	Yellow  = "\033[33m"
	Blue    = "\033[34m"
	Magenta = "\033[35m"
	Cyan    = "\033[36m"
	White   = "\033[37m"
	Gray    = "\033[90m"
)

// Status icons
var StatusIcons = map[string]string{
	"pending":     "○",
	"in_progress": "◐",
	"review":      "◑",
	"done":        "●",
	"blocked":     "✗",
}

// Status colors
var StatusColors = map[string]string{
	"pending":     Gray,
	"in_progress": Yellow,
	"review":      Blue,
	"done":        Green,
	"blocked":     Red,
}

// Type icons
var TypeIcons = map[string]string{
	"feature":  "✚",
	"bug":      "🐛",
	"docs":     "📄",
	"refactor": "♻",
	"test":     "🧪",
	"chore":    "⚙",
}

// Type colors
var TypeColors = map[string]string{
	"feature":  Green,
	"bug":      Red,
	"docs":     Blue,
	"refactor": Yellow,
	"test":     Magenta,
	"chore":    Gray,
}

// Priority colors
var PriorityColors = map[string]string{
	"critical": Red,
	"high":     Yellow,
	"medium":   Cyan,
	"low":      Gray,
}

// Phase status icons
var PhaseStatusIcons = map[string]string{
	"pending":     "◇",
	"defining":    "✎",
	"ready":       "○",
	"in_progress": "◐",
	"done":        "●",
}

// Phase status colors
var PhaseStatusColors = map[string]string{
	"pending":     Gray,
	"defining":    Magenta,
	"ready":       Cyan,
	"in_progress": Yellow,
	"done":        Green,
}

// ColorStatus returns a colored status string with icon
func ColorStatus(s string) string {
	color := StatusColors[s]
	icon := StatusIcons[s]
	if icon == "" {
		icon = "?"
	}
	return fmt.Sprintf("%s%s %s%s", color, icon, s, Reset)
}

// ColorType returns a colored type string with icon
func ColorType(t string) string {
	color := TypeColors[t]
	icon := TypeIcons[t]
	if icon == "" {
		icon = "?"
	}
	return fmt.Sprintf("%s%s %s%s", color, icon, t, Reset)
}

// ColorPriority returns a colored priority string
func ColorPriority(p string) string {
	color := PriorityColors[p]
	return fmt.Sprintf("%s%s%s", color, p, Reset)
}

// ColorPhaseStatus returns a colored phase status with icon
func ColorPhaseStatus(s string) string {
	color := PhaseStatusColors[s]
	icon := PhaseStatusIcons[s]
	if icon == "" {
		icon = "?"
	}
	return fmt.Sprintf("%s%s %s%s", color, icon, s, Reset)
}

// ColorID returns a colored task ID
func ColorID(id string) string {
	return fmt.Sprintf("%s%s%s", Cyan, id, Reset)
}

// ProgressBar renders a progress bar
func ProgressBar(done, total, width int) string {
	if total == 0 {
		return fmt.Sprintf("%s%s 0%%%s", Gray, strings.Repeat("░", width), Reset)
	}
	filled := (done * width) / total
	if filled > width {
		filled = width
	}
	pct := (done * 100) / total
	var color string
	if pct == 100 {
		color = Green
	} else if pct >= 50 {
		color = Yellow
	} else {
		color = Gray
	}
	return fmt.Sprintf("%s%s%s%s%s %d%% (%d/%d)",
		color, strings.Repeat("█", filled),
		Gray, strings.Repeat("░", width-filled),
		Reset, pct, done, total)
}

// RenderMarkdown applies basic terminal markdown rendering
func RenderMarkdown(text string) string {
	lines := strings.Split(text, "\n")
	var result []string

	for _, line := range lines {
		// Headers
		if strings.HasPrefix(line, "#### ") {
			title := line[5:]
			result = append(result, fmt.Sprintf("%s▹%s %s%s%s", Yellow, Reset, Bold, title, Reset))
			continue
		}
		if strings.HasPrefix(line, "### ") {
			title := line[4:]
			result = append(result, fmt.Sprintf("%s▸%s %s%s%s%s", Magenta, Reset, Bold, Magenta, title, Reset))
			continue
		}
		if strings.HasPrefix(line, "## ") {
			title := line[3:]
			result = append(result, fmt.Sprintf("\n%s│%s %s%s%s%s", Blue, Reset, Bold, Blue, title, Reset))
			continue
		}
		if strings.HasPrefix(line, "# ") {
			title := line[2:]
			result = append(result, fmt.Sprintf("\n%s┃%s %s%s%s%s", Cyan, Reset, Bold, Cyan, title, Reset))
			continue
		}

		// Horizontal rule
		if len(line) >= 3 && strings.TrimRight(line, "-") == "" {
			result = append(result, fmt.Sprintf("%s%s%s", Dim, strings.Repeat("─", 40), Reset))
			continue
		}

		// Checklist items
		if idx := strings.Index(line, "- [x] "); idx >= 0 {
			prefix := line[:idx]
			text := line[idx+6:]
			line = fmt.Sprintf("%s%s✓%s %s%s%s", prefix, Green, Reset, Dim, text, Reset)
		} else if idx := strings.Index(line, "- [X] "); idx >= 0 {
			prefix := line[:idx]
			text := line[idx+6:]
			line = fmt.Sprintf("%s%s✓%s %s%s%s", prefix, Green, Reset, Dim, text, Reset)
		} else if idx := strings.Index(line, "- [ ] "); idx >= 0 {
			prefix := line[:idx]
			text := line[idx+6:]
			line = fmt.Sprintf("%s%s☐%s %s", prefix, Gray, Reset, text)
		}

		// Unordered list bullets
		if trimmed := line; len(trimmed) > 0 {
			for i, ch := range line {
				if ch == '-' && i+1 < len(line) && line[i+1] == ' ' {
					// Check if prefix is only spaces
					prefix := line[:i]
					if strings.TrimSpace(prefix) == "" {
						line = prefix + fmt.Sprintf("%s•%s ", Dim, Reset) + line[i+2:]
					}
					break
				}
				if ch != ' ' && ch != '\t' {
					break
				}
			}
		}

		// Inline formatting: bold
		for {
			start := strings.Index(line, "**")
			if start == -1 {
				break
			}
			end := strings.Index(line[start+2:], "**")
			if end == -1 {
				break
			}
			end += start + 2
			inner := line[start+2 : end]
			line = line[:start] + Bold + inner + Reset + line[end+2:]
		}

		// Inline formatting: italic (single *)
		for {
			start := strings.Index(line, "*")
			if start == -1 {
				break
			}
			end := strings.Index(line[start+1:], "*")
			if end == -1 {
				break
			}
			end += start + 1
			inner := line[start+1 : end]
			line = line[:start] + Dim + inner + Reset + line[end+1:]
		}

		// Inline code
		for {
			start := strings.Index(line, "`")
			if start == -1 {
				break
			}
			end := strings.Index(line[start+1:], "`")
			if end == -1 {
				break
			}
			end += start + 1
			inner := line[start+1 : end]
			line = line[:start] + Yellow + inner + Reset + line[end+1:]
		}

		result = append(result, line)
	}

	return strings.Join(result, "\n")
}
