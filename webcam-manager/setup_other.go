//go:build !windows

package main

import "errors"

func interactiveSetup(string) bool            { return false }
func launchedByDoubleClick() bool             { return false }
func startAndOpen(string) error               { return errors.New("solo su Windows") }
func setupInstall(string, string, bool) error { return errors.New("solo su Windows") }
