package tui

import (
	"os"
	"strings"
	"sync"
)

// ANSI color escape codes for terminal styling.
const (
	Reset       = "\033[0m"
	Bold        = "\033[1m"
	Dim         = "\033[2m"
	Italic      = "\033[3m"
	Underline   = "\033[4m"
	ClearScreen = "\033[2J\033[H"

	// Standard 16-color ANSI
	Red     = "\033[31m"
	Green   = "\033[32m"
	Yellow  = "\033[33m"
	Blue    = "\033[34m"
	Magenta = "\033[35m"
	Cyan    = "\033[36m"
	White   = "\033[37m"

	// 256-color / High-intensity codes
	HiBlack  = "\033[90m"
	HiRed    = "\033[91m"
	HiGreen  = "\033[92m"
	HiYellow = "\033[93m"
	HiBlue   = "\033[94m"
	HiPurple = "\033[95m"
	HiCyan   = "\033[96m"
	HiWhite  = "\033[97m"

	// 24-bit TrueColor / 256-color approximations for specific palettes
	MonokaiGreen  = "\033[38;5;148m"
	MonokaiYellow = "\033[38;5;221m"
	MonokaiPink   = "\033[38;5;197m"
	MonokaiPurple = "\033[38;5;141m"
	MonokaiCyan   = "\033[38;5;81m"

	DraculaPurple = "\033[38;5;141m"
	DraculaPink   = "\033[38;5;212m"
	DraculaCyan   = "\033[38;5;117m"
	DraculaGreen  = "\033[38;5;84m"
	DraculaOrange = "\033[38;5;215m"
	DraculaRed    = "\033[38;5;203m"

	GruvboxOrange = "\033[38;5;208m"
	GruvboxYellow = "\033[38;5;214m"
	GruvboxGreen  = "\033[38;5;142m"
	GruvboxAqua   = "\033[38;5;108m"
	GruvboxBlue   = "\033[38;5;109m"
	GruvboxPurple = "\033[38;5;175m"
	GruvboxRed    = "\033[38;5;167m"

	NordFrost1 = "\033[38;5;110m"
	NordFrost2 = "\033[38;5;111m"
	NordFrost3 = "\033[38;5;152m"
	NordSnow   = "\033[38;5;255m"
	NordAurora = "\033[38;5;150m"
	NordRed    = "\033[38;5;203m"
)

// Theme encapsulates semantic color codes for terminal visual rendering.
type Theme struct {
	Name    string `json:"name"`
	Border  string `json:"border"`
	Header  string `json:"header"`
	Accent  string `json:"accent"`
	Text    string `json:"text"`
	DimText string `json:"dim_text"`
	Success string `json:"success"`
	Warning string `json:"warning"`
	Error   string `json:"error"`
	Info    string `json:"info"`
	Prompt  string `json:"prompt"`
}

var (
	themeRegistryMu sync.RWMutex
	themeRegistry   = make(map[string]Theme)
)

func init() {
	RegisterTheme(DefaultTheme())
	RegisterTheme(MonokaiTheme())
	RegisterTheme(DraculaTheme())
	RegisterTheme(GruvboxTheme())
	RegisterTheme(NordTheme())
}

// DefaultTheme provides crisp classic ANSI terminal styling.
func DefaultTheme() Theme {
	return Theme{
		Name:    "default",
		Border:  Cyan,
		Header:  Cyan,
		Accent:  Magenta,
		Text:    Reset,
		DimText: Dim,
		Success: Green,
		Warning: Yellow,
		Error:   Red,
		Info:    Blue,
		Prompt:  HiCyan,
	}
}

// MonokaiTheme provides high-contrast vibrant colors based on the Monokai palette.
func MonokaiTheme() Theme {
	return Theme{
		Name:    "monokai",
		Border:  MonokaiCyan,
		Header:  MonokaiPink,
		Accent:  MonokaiPurple,
		Text:    Reset,
		DimText: HiBlack,
		Success: MonokaiGreen,
		Warning: MonokaiYellow,
		Error:   MonokaiPink,
		Info:    MonokaiCyan,
		Prompt:  MonokaiYellow,
	}
}

// DraculaTheme provides gothic dark-mode aesthetic with vibrant pastel accents.
func DraculaTheme() Theme {
	return Theme{
		Name:    "dracula",
		Border:  DraculaPurple,
		Header:  DraculaPink,
		Accent:  DraculaCyan,
		Text:    Reset,
		DimText: HiBlack,
		Success: DraculaGreen,
		Warning: DraculaOrange,
		Error:   DraculaRed,
		Info:    DraculaCyan,
		Prompt:  DraculaPurple,
	}
}

// GruvboxTheme provides warm retro tones designed for extended viewing.
func GruvboxTheme() Theme {
	return Theme{
		Name:    "gruvbox",
		Border:  GruvboxAqua,
		Header:  GruvboxOrange,
		Accent:  GruvboxPurple,
		Text:    Reset,
		DimText: HiBlack,
		Success: GruvboxGreen,
		Warning: GruvboxYellow,
		Error:   GruvboxRed,
		Info:    GruvboxBlue,
		Prompt:  GruvboxOrange,
	}
}

// NordTheme provides arctic frost tones.
func NordTheme() Theme {
	return Theme{
		Name:    "nord",
		Border:  NordFrost1,
		Header:  NordFrost2,
		Accent:  NordFrost3,
		Text:    NordSnow,
		DimText: HiBlack,
		Success: NordAurora,
		Warning: NordFrost2,
		Error:   NordRed,
		Info:    NordFrost1,
		Prompt:  NordFrost3,
	}
}

// RegisterTheme stores a theme in the global registry.
func RegisterTheme(t Theme) {
	themeRegistryMu.Lock()
	defer themeRegistryMu.Unlock()
	themeRegistry[strings.ToLower(t.Name)] = t
}

// GetTheme retrieves a theme by name, falling back to DefaultTheme if not found.
func GetTheme(name string) Theme {
	themeRegistryMu.RLock()
	defer themeRegistryMu.RUnlock()
	if t, exists := themeRegistry[strings.ToLower(name)]; exists {
		return t
	}
	return DefaultTheme()
}

// ListThemes returns the sorted list of available theme names.
func ListThemes() []string {
	themeRegistryMu.RLock()
	defer themeRegistryMu.RUnlock()
	list := make([]string, 0, len(themeRegistry))
	for name := range themeRegistry {
		list = append(list, name)
	}
	return list
}

// LoadThemeFromEnv checks AGENTVCR_THEME environment variable.
func LoadThemeFromEnv() Theme {
	env := os.Getenv("AGENTVCR_THEME")
	if env != "" {
		return GetTheme(env)
	}
	return DefaultTheme()
}
