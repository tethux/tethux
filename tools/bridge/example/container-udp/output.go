package main

import (
	"io"
	"regexp"
	"strings"
)

var tomlToken = regexp.MustCompile(`(?m)^\s*\[\[?[^\r\n]*?\]\]?|^\s*[A-Za-z_][A-Za-z_0-9]*\s*=|'[^']*'|"(?:\\.|[^"\\])*"|\b(?:true|false|[0-9]+)\b`)

func printTOML(output io.Writer, document []byte, color bool) error {
	text := string(document)
	if color {
		text = tomlToken.ReplaceAllStringFunc(text, func(token string) string {
			code := "\x1b[36m"
			value := strings.TrimSpace(token)
			switch {
			case strings.HasPrefix(value, "["):
				code = "\x1b[1;35m"
			case strings.HasPrefix(value, "'") || strings.HasPrefix(value, "\""):
				code = "\x1b[32m"
			case strings.HasSuffix(value, "="):
				code = "\x1b[33m"
			}
			return code + token + "\x1b[0m"
		})
	}
	_, err := io.WriteString(output, "\nTopology TOML (test input):\n\n"+text+"\n")
	return err
}
