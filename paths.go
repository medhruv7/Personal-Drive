package main

import (
	"errors"
	"path"
	"path/filepath"
	"strings"
)

// errBadRequest marks errors caused by invalid client input (mapped to 400).
var errBadRequest = errors.New("bad request")

type badRequestError struct{ msg string }

func (e badRequestError) Error() string { return e.msg }
func (e badRequestError) Is(target error) bool {
	return target == errBadRequest
}

func badRequest(msg string) error { return badRequestError{msg} }

// resolvePath turns an API path like "/photos/2024" into a clean, OS-specific
// path relative to the drive root ("." for the root itself). Lexical ".."
// segments are neutralised by path.Clean; os.Root additionally refuses any
// symlink that escapes the root.
func resolvePath(p string) (string, error) {
	if strings.ContainsRune(p, 0) || strings.Contains(p, `\`) {
		return "", badRequest("invalid path")
	}
	rel := strings.TrimPrefix(path.Clean("/"+p), "/")
	if rel == "" {
		return ".", nil
	}
	osRel := filepath.FromSlash(rel)
	if !filepath.IsLocal(osRel) {
		return "", badRequest("invalid path")
	}
	return osRel, nil
}

// canonicalPath returns the slash-separated form of p that the UI displays.
func canonicalPath(p string) string {
	return path.Clean("/" + p)
}

var windowsReserved = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
	"COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
	"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// validateName checks a single file or folder name. The rules are the union
// of macOS and Windows restrictions so the drive stays portable between them.
func validateName(name string) error {
	if name == "" || name == "." || name == ".." {
		return badRequest("invalid name")
	}
	if len(name) > 255 {
		return badRequest("name too long")
	}
	for _, r := range name {
		if r < 0x20 || strings.ContainsRune(`/\:*?"<>|`, r) {
			return badRequest(`name cannot contain / \ : * ? " < > | or control characters`)
		}
	}
	if strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		return badRequest("name cannot end with a dot or space")
	}
	stem, _, _ := strings.Cut(name, ".")
	if windowsReserved[strings.ToUpper(strings.TrimSpace(stem))] {
		return badRequest("name is reserved on Windows")
	}
	return nil
}

// uploadName reduces a client-supplied filename to its base name. Some
// browsers send a full path, using either separator.
func uploadName(raw string) string {
	if i := strings.LastIndexAny(raw, `/\`); i >= 0 {
		raw = raw[i+1:]
	}
	return strings.TrimSpace(raw)
}

// isHidden reports whether a name is hidden from listings by default.
func isHidden(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	switch strings.ToLower(name) {
	case "desktop.ini", "thumbs.db", "$recycle.bin", "system volume information":
		return true
	}
	return false
}
