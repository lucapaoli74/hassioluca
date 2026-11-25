package main

import (
	"errors"

	"github.com/go-ole/go-ole"
	"tinygo.org/x/bluetooth"
)

func bleEnable(a *bluetooth.Adapter) error {
	err := a.Enable()
	var oe *ole.OleError
	// S_FALSE: il thread era già inizializzato, va bene così
	if errors.As(err, &oe) && oe.Code() == 1 {
		return nil
	}
	return err
}
