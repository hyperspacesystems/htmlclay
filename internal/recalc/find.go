package recalc

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// FindBrowser returns the path to an installed Chromium-family browser, or "".
// Setting HTMLCLAY_RECALC_BROWSER overrides discovery; "none" disables it.
func FindBrowser() string {
	if env := strings.TrimSpace(os.Getenv("HTMLCLAY_RECALC_BROWSER")); env != "" {
		if strings.EqualFold(env, "none") {
			return ""
		}
		if isExecutableFile(env) {
			return env
		}
		return ""
	}
	return findInstalledBrowser(runtime.GOOS)
}

func findInstalledBrowser(goos string) string {
	switch goos {
	case "darwin":
		home, _ := os.UserHomeDir()
		relApps := []string{
			"Google Chrome.app/Contents/MacOS/Google Chrome",
			"Brave Browser.app/Contents/MacOS/Brave Browser",
			"Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"Chromium.app/Contents/MacOS/Chromium",
		}
		var paths []string
		for _, rel := range relApps {
			paths = append(paths, filepath.Join("/Applications", rel))
		}
		if home != "" {
			for _, rel := range relApps {
				paths = append(paths, filepath.Join(home, "Applications", rel))
			}
		}
		for _, p := range paths {
			if isExecutableFile(p) {
				return p
			}
		}
		return lookPathAny("google-chrome", "chromium", "chromium-browser", "brave-browser", "microsoft-edge")
	case "linux":
		// Prefer native installs ahead of chromium/chromium-browser (which on Ubuntu wrap the Snap)
		// and skip Flatpak export wrappers (which drop inherited fds 3/4 and spawn outside our pgrp).
		if p := lookPathLinux(
			"google-chrome",
			"google-chrome-stable",
			"brave-browser",
			"microsoft-edge",
			"microsoft-edge-stable",
			"chromium",
			"chromium-browser",
		); p != "" {
			return p
		}
		for _, p := range []string{
			"/usr/bin/google-chrome",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/brave-browser",
			"/usr/bin/microsoft-edge",
			"/usr/bin/microsoft-edge-stable",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
			"/snap/bin/chromium",
		} {
			if isExecutableFile(p) {
				return p
			}
		}
		return ""
	case "windows":
		var roots []string
		for _, envKey := range []string{"ProgramFiles", "ProgramFiles(x86)", "LocalAppData"} {
			if v := os.Getenv(envKey); v != "" {
				roots = append(roots, v)
			}
		}
		rels := []string{
			filepath.Join("Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join("Microsoft", "Edge", "Application", "msedge.exe"),
			filepath.Join("BraveSoftware", "Brave-Browser", "Application", "brave.exe"),
		}
		for _, root := range roots {
			for _, rel := range rels {
				p := filepath.Join(root, rel)
				if isExecutableFile(p) {
					return p
				}
			}
		}
		return lookPathAny("chrome.exe", "msedge.exe", "brave.exe", "chromium.exe")
	default:
		return ""
	}
}

func lookPathAny(names ...string) string {
	for _, name := range names {
		if p, err := exec.LookPath(name); err == nil && p != "" {
			return p
		}
	}
	return ""
}

func lookPathLinux(names ...string) string {
	for _, name := range names {
		if p, err := exec.LookPath(name); err == nil && p != "" && !strings.Contains(p, "/flatpak/exports/bin/") {
			return p
		}
	}
	return ""
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode()&0o111 != 0
}

func headlessFlag(bin string) string {
	base := strings.ToLower(filepath.Base(bin))
	if strings.Contains(base, "headless-shell") || strings.Contains(base, "headless_shell") {
		return "--headless"
	}
	return "--headless=new"
}
