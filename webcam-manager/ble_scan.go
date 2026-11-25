//go:build windows || linux

package main

import (
	"context"
	"runtime"
	"time"

	"tinygo.org/x/bluetooth"
)

var bthomeUUID = bluetooth.New16BitUUID(0xFCD2)

// bleScan ascolta le trasmissioni Bluetooth Low Energy finché ctx non è annullato.
func bleScan(ctx context.Context, cb func(bleAdv)) error {
	// su Windows il Bluetooth (WinRT) va inizializzato sul thread che lo usa
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a := bluetooth.DefaultAdapter
	if err := bleEnable(a); err != nil {
		return err
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-done:
			return
		case <-ctx.Done():
		}
		// la ricerca può non essere ancora partita: si riprova finché si ferma
		for a.StopScan() != nil {
			select {
			case <-done:
				return
			case <-time.After(200 * time.Millisecond):
			}
		}
	}()
	err := a.Scan(func(_ *bluetooth.Adapter, r bluetooth.ScanResult) {
		adv := bleAdv{Address: r.Address.String(), Name: r.LocalName(), RSSI: int(r.RSSI)}
		for _, sd := range r.ServiceData() {
			if sd.UUID == bthomeUUID {
				adv.BTHome = sd.Data
			}
		}
		cb(adv)
	})
	close(done)
	if ctx.Err() != nil {
		return nil
	}
	return err
}
