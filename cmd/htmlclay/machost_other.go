//go:build !darwin

package main

func bundledMacHost() (string, bool) { return "", false }

func handOffToMacApp(shellStdin, noTray bool, args []string) (int, bool) { return 0, false }
