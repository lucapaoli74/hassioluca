package main

import (
	"context"
	"io"
	"net"
	"testing"
)

// fakeConfigConsole simula i comandi di configurazione di una console Ecowitt.
func fakeConfigConsole(t *testing.T) string {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	custom, path := packEcowittCustom(EcowittCustom{Station: "st", Key: "k", Server: "1.2.3.4", Port: 80, Interval: 300, Protocol: "wunderground"})
	reply := func(c net.Conn, cmd byte, data []byte) {
		b := append([]byte{cmd, byte(len(data) + 3)}, data...)
		var sum byte
		for _, x := range b {
			sum += x
		}
		c.Write(append(append([]byte{0xFF, 0xFF}, b...), sum))
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			hdr := make([]byte, 4)
			if _, err := io.ReadFull(c, hdr); err != nil {
				c.Close()
				continue
			}
			rest := make([]byte, int(hdr[3])-2)
			io.ReadFull(c, rest)
			data := rest[:len(rest)-1]
			switch hdr[2] {
			case cmdReadFirmware:
				v := "EasyWeatherV1.6.4"
				reply(c, hdr[2], append([]byte{byte(len(v))}, v...))
			case cmdReadCustomized:
				reply(c, hdr[2], custom)
			case cmdReadUserPath:
				reply(c, hdr[2], path)
			case cmdWriteCustom:
				custom = append([]byte(nil), data...)
				reply(c, hdr[2], []byte{0})
			case cmdWriteUserPath:
				path = append([]byte(nil), data...)
				reply(c, hdr[2], []byte{0})
			}
			c.Close()
		}
	}()
	return ln.Addr().String()
}

func TestConfigureEcowittConsole(t *testing.T) {
	addr := fakeConfigConsole(t)
	info := ReadEcowittConsole(context.Background(), addr)
	if info.Custom == nil || info.Firmware != "EasyWeatherV1.6.4" || info.Custom.Server != "1.2.3.4" || info.Custom.Protocol != "wunderground" {
		t.Fatalf("%+v %+v", info, info.Custom)
	}
	c, err := ConfigureEcowittConsole(context.Background(), addr, "192.168.1.10", 8080, 60)
	if err != nil {
		t.Fatal(err)
	}
	if c.Server != "192.168.1.10" || c.Port != 8080 || !c.Enabled || c.Protocol != "ecowitt" || c.PathEcowitt != "/data/report/" || c.Station != "st" || c.Interval != 60 {
		t.Fatalf("%+v", c)
	}
}

func TestParseEcowittCustomRejectsUnknownFormat(t *testing.T) {
	cb, pb := packEcowittCustom(EcowittCustom{Server: "x"})
	if _, err := ParseEcowittCustom(append(cb, 9), pb); err == nil {
		t.Fatal("formato sbagliato accettato")
	}
}
