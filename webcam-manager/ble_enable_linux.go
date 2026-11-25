package main

import "tinygo.org/x/bluetooth"

func bleEnable(a *bluetooth.Adapter) error { return a.Enable() }
