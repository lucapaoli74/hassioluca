package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const liveHTM = `<html><head><title>LiveData</title></head><body><form>
<tr><td>Outdoor Temperature</td><td><input name="outTemp" disabled="disabled" type="text" class="item_2" value="12.3" maxlength="5" /></td></tr>
<input type="text" value="78" name="outHumi"><input name="RelPress" value="1013.40" type="text">
<input name="windir" value="225"><input name="avgwind" value="2.5"><input name="gustspeed" value="5.0">
<input name="rainofdaily" value="1.20"><input name="solarrad" value="412.5"><input name="uvi" value="3">
<input name="inTemp" value="----"></form></body></html>`

const stationHTM = `<select name="unit_Wind" class="item_1"><option value="0" selected="selected">m/s</option><option value="1">km/h</option></select>
<select name="u_Rainfall"><option value="0" selected>mm</option></select><select name="unit_Temperature"><option value="0" selected="selected">degC</option></select>`

func TestObserverLive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/livedata.htm":
			w.Write([]byte(liveHTM))
		case "/station.htm":
			w.Write([]byte(stationHTM))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	v, err := readObserverLive(context.Background(), strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]float64{"temp": 12.3, "humidity": 78, "pressure": 1013.4, "winddir": 225, "wind": 9, "gust": 18, "rain_day": 1.2, "solar": 412.5, "uv": 3}
	for k, w := range want {
		if v[k] != w {
			t.Errorf("%s = %v, atteso %v", k, v[k], w)
		}
	}
	if _, ok := v["temp_in"]; ok {
		t.Error("campo vuoto ---- letto come valore")
	}
	// unità imperiali
	u := observerUnits{temp: "degf", wind: "mph", press: "inhg", rain: "in"}
	iv := ParseObserverLive(`<input name="outTemp" value="50"><input name="avgwind" value="10"><input name="RelPress" value="29.92"><input name="rainofdaily" value="1">`, u)
	if iv["temp"] != 10 || iv["wind"] != 16.1 || iv["pressure"] != 1013.2 || iv["rain_day"] != 25.4 {
		t.Fatal(iv)
	}
}
