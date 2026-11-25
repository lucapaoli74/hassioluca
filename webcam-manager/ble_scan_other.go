//go:build !windows && !linux

package main

import (
	"context"
	"errors"
)

func bleScan(ctx context.Context, cb func(bleAdv)) error {
	return errors.New("il Bluetooth non è supportato su questo sistema operativo")
}
