//go:build windows

package main

import "golang.org/x/sys/windows"

func systemLanguageTag() string {
	languages, err := windows.GetUserPreferredUILanguages(windows.MUI_LANGUAGE_NAME)
	if err != nil || len(languages) == 0 {
		return ""
	}
	return languages[0]
}
