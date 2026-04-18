package ui

import (
	"fmt"
	"strings"
	"sync/atomic"
)

const (
	colorReset  = "\033[0m"
	colorBlue   = "\033[34m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
	colorCyan   = "\033[36m"
)

type Logger struct{}

var debugEnabled atomic.Bool

func NewLogger() Logger {
	return Logger{}
}

func SetDebug(enabled bool) {
	debugEnabled.Store(enabled)
}

func DebugEnabled() bool {
	return debugEnabled.Load()
}

func Infof(format string, args ...interface{}) {
	NewLogger().Infof(format, args...)
}

func Successf(format string, args ...interface{}) {
	NewLogger().Successf(format, args...)
}

func Warnf(format string, args ...interface{}) {
	NewLogger().Warnf(format, args...)
}

func Errorf(format string, args ...interface{}) {
	NewLogger().Errorf(format, args...)
}

func Debugf(format string, args ...interface{}) {
	NewLogger().Debugf(format, args...)
}

func (Logger) Infof(format string, args ...interface{}) {
	fmt.Printf(colorBlue+"[info] "+format+colorReset+"\n", args...)
}

func (Logger) Successf(format string, args ...interface{}) {
	fmt.Printf(colorGreen+"[ok] "+format+colorReset+"\n", args...)
}

func (Logger) Warnf(format string, args ...interface{}) {
	fmt.Printf(colorYellow+"[warn] "+format+colorReset+"\n", args...)
}

func (Logger) Errorf(format string, args ...interface{}) {
	fmt.Printf(colorRed+"[error] "+format+colorReset+"\n", args...)
}

func (Logger) Debugf(format string, args ...interface{}) {
	if !DebugEnabled() {
		return
	}
	fmt.Printf(colorCyan+"[debug] "+format+colorReset+"\n", args...)
}

func PrintTable(headers []string, rows [][]string) {
	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = len(header)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	fmt.Println(formatRow(headers, widths))
	separators := make([]string, len(headers))
	for i := range headers {
		separators[i] = strings.Repeat("-", widths[i])
	}
	fmt.Println(formatRow(separators, widths))

	for _, row := range rows {
		fmt.Println(formatRow(row, widths))
	}
	fmt.Println()
}

func PrintSolidTable(headers []string, rows [][]string) {
	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = len(header)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	topBorder := buildSolidBorder(widths)
	fmt.Println(topBorder)
	fmt.Println(formatSolidRow(headers, widths))
	fmt.Println(topBorder)
	for _, row := range rows {
		fmt.Println(formatSolidRow(row, widths))
	}
	fmt.Println(topBorder)
	fmt.Println()
}

func formatRow(values []string, widths []int) string {
	out := make([]string, len(widths))
	for i := range widths {
		value := ""
		if i < len(values) {
			value = values[i]
		}
		out[i] = fmt.Sprintf("%-*s", widths[i], value)
	}
	return strings.Join(out, "  ")
}

func formatSolidRow(values []string, widths []int) string {
	parts := make([]string, 0, len(widths))
	for i := range widths {
		value := ""
		if i < len(values) {
			value = values[i]
		}
		parts = append(parts, fmt.Sprintf(" %-*s ", widths[i], value))
	}
	return "|" + strings.Join(parts, "|") + "|"
}

func buildSolidBorder(widths []int) string {
	parts := make([]string, 0, len(widths))
	for _, width := range widths {
		parts = append(parts, strings.Repeat("-", width+2))
	}
	return "+" + strings.Join(parts, "+") + "+"
}
