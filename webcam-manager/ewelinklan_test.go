package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"encoding/base64"
	"net"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

func ewelinkEncryptForTest(key string, plain []byte) (string, string) {
	sum := md5.Sum([]byte(key))
	iv := []byte("0123456789abcdef")
	pad := aes.BlockSize - len(plain)%aes.BlockSize
	for i := 0; i < pad; i++ {
		plain = append(plain, byte(pad))
	}
	block, _ := aes.NewCipher(sum[:])
	out := make([]byte, len(plain))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(out, plain)
	return base64.StdEncoding.EncodeToString(iv), base64.StdEncoding.EncodeToString(out)
}

func TestEWeLinkLAN(t *testing.T) {
	l := &ewelinkLAN{keys: map[string]string{}, uiid: map[string]int{}, values: map[string]lanReading{}, seen: map[string]time.Time{}}
	l.setKeysPath(filepath.Join(t.TempDir(), "k.json"))
	l.learn([]ewelinkThing{{ID: "bridge1", DeviceKey: "secret-key", UIID: 128}, {ID: "sens1", UIID: 1770}})
	iv, data := ewelinkEncryptForTest("secret-key", []byte(`{"subDevId":"sens1","temperature":"2156","humidity":"5400"}`))
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{Response: true})
	_ = b.StartAnswers()
	name := dnsmessage.MustNewName("eWeLink_bridge1._ewelink._tcp.local.")
	_ = b.TXTResource(dnsmessage.ResourceHeader{Name: name, Class: dnsmessage.ClassINET, TTL: 120},
		dnsmessage.TXTResource{TXT: []string{"txtvers=1", "id=bridge1", "type=diy_plug", "encrypt=true", "iv=" + iv, "data1=" + data[:len(data)/2], "data2=" + data[len(data)/2:]}})
	pkt, _ := b.Finish()
	l.handle(pkt, &net.UDPAddr{IP: net.IPv4(192, 168, 1, 50)})
	if v, ok := l.value("sens1", "temperature", time.Minute); !ok || v != 21.56 {
		t.Fatal("temperatura LAN", v, ok)
	}
	if v, ok := l.value("sens1", "humidity", time.Minute); !ok || v != 54 {
		t.Fatal("umidità LAN", v, ok)
	}
	// le chiavi restano salvate
	l2 := &ewelinkLAN{keys: map[string]string{}, uiid: map[string]int{}, values: map[string]lanReading{}, seen: map[string]time.Time{}}
	l2.setKeysPath(l.keysPath)
	if l2.keys["bridge1"] != "secret-key" || l2.uiid["sens1"] != 1770 {
		t.Fatal("chiavi non salvate", l2.keys, l2.uiid)
	}
	if _, err := ewelinkDecrypt("chiave-sbagliata", iv, data); err == nil {
		t.Log("nota: una chiave sbagliata può superare il controllo del padding; il JSON non sarà valido")
	}
}
