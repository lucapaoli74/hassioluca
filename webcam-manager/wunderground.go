package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"time"
)

// Ricerca delle stazioni meteo personali (PWS) di Weather Underground e
// lettura dei loro valori, per sceglierli dal pannello.

// WUStation è una stazione trovata nelle vicinanze.
type WUStation struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	DistanceKm float64 `json:"distance_km"`
	Latitude   float64 `json:"latitude"`
	Longitude  float64 `json:"longitude"`
}

// WUValue è un valore attuale di una stazione.
type WUValue struct {
	Variable string  `json:"variable"`
	Label    string  `json:"label"`
	Value    float64 `json:"value"`
	Unit     string  `json:"unit"`
}

// wuVariables: nome del campo, etichetta, unità (in ordine di visualizzazione).
var wuVariables = []struct{ key, label, unit string }{
	{"temp", "Temperatura", "°C"}, {"humidity", "Umidità", "%"}, {"dewpt", "Punto di rugiada", "°C"},
	{"heatIndex", "Indice di calore", "°C"}, {"windChill", "Temperatura percepita (vento)", "°C"},
	{"windSpeed", "Vento", " km/h"}, {"windGust", "Raffiche", " km/h"}, {"winddir", "Direzione vento", "°"},
	{"pressure", "Pressione", " hPa"}, {"precipRate", "Intensità pioggia", " mm/h"}, {"precipTotal", "Pioggia oggi", " mm"},
	{"uv", "Indice UV", ""}, {"solarRadiation", "Radiazione solare", " W/m²"},
}

func wuGet(ctx context.Context, endpoint string, q url.Values, key string) (any, error) {
	if key == "" {
		return nil, errors.New("serve la chiave API di Weather Underground")
	}
	q.Set("apiKey", key)
	q.Set("format", "json")
	doc, err := getJSON(ctx, "https://api.weather.com"+endpoint+"?"+q.Encode(), "")
	if err != nil {
		return nil, fmt.Errorf("Weather Underground: %w (chiave API corretta?)", err)
	}
	return doc, nil
}

// WUGeocode trova le coordinate di una località.
func WUGeocode(ctx context.Context, key, query string) (float64, float64, string, error) {
	q := url.Values{}
	q.Set("query", query)
	q.Set("language", "it-IT")
	doc, err := wuGet(ctx, "/v3/location/search", q, key)
	if err != nil {
		return 0, 0, "", err
	}
	lats, _ := lookupPath(doc, "location.latitude")
	lons, _ := lookupPath(doc, "location.longitude")
	addr, _ := lookupPath(doc, "location.address")
	la, _ := lats.([]any)
	lo, _ := lons.([]any)
	ad, _ := addr.([]any)
	if len(la) == 0 || len(lo) == 0 {
		return 0, 0, "", fmt.Errorf("località %q non trovata", query)
	}
	lat, _ := la[0].(float64)
	lon, _ := lo[0].(float64)
	name := query
	if len(ad) > 0 {
		if s, ok := ad[0].(string); ok {
			name = s
		}
	}
	return lat, lon, name, nil
}

// WUNearby elenca le stazioni personali vicine alle coordinate.
func WUNearby(ctx context.Context, key string, lat, lon float64) ([]WUStation, error) {
	q := url.Values{}
	q.Set("geocode", strconv.FormatFloat(lat, 'f', 4, 64)+","+strconv.FormatFloat(lon, 'f', 4, 64))
	q.Set("product", "pws")
	doc, err := wuGet(ctx, "/v3/location/near", q, key)
	if err != nil {
		return nil, err
	}
	col := func(name string) []any {
		v, _ := lookupPath(doc, "location."+name)
		a, _ := v.([]any)
		return a
	}
	ids, names, dist, lats, lons := col("stationId"), col("stationName"), col("distanceKm"), col("latitude"), col("longitude")
	out := []WUStation{}
	for i := range ids {
		st := WUStation{}
		st.ID, _ = ids[i].(string)
		if i < len(names) {
			st.Name, _ = names[i].(string)
		}
		if i < len(dist) {
			st.DistanceKm, _ = dist[i].(float64)
		}
		if i < len(lats) {
			st.Latitude, _ = lats[i].(float64)
		}
		if i < len(lons) {
			st.Longitude, _ = lons[i].(float64)
		}
		if st.ID != "" {
			out = append(out, st)
		}
	}
	return out, nil
}

// WUObservation legge tutti i valori attuali di una stazione.
func WUObservation(ctx context.Context, key, station string) ([]WUValue, time.Time, error) {
	q := url.Values{}
	q.Set("stationId", station)
	q.Set("units", "m")
	q.Set("numericPrecision", "decimal")
	doc, err := wuGet(ctx, "/v2/pws/observations/current", q, key)
	if err != nil {
		return nil, time.Time{}, err
	}
	var when time.Time
	if s, err := lookupPath(doc, "observations.0.obsTimeUtc"); err == nil {
		if str, ok := s.(string); ok {
			when, _ = time.Parse(time.RFC3339, str)
		}
	}
	out := []WUValue{}
	for _, v := range wuVariables {
		p := "observations.0." + v.key
		if wuMetric[v.key] {
			p = "observations.0.metric." + v.key
		}
		raw, err := lookupPath(doc, p)
		if err != nil {
			continue
		}
		f, ok := raw.(float64)
		if !ok || math.IsNaN(f) {
			continue
		}
		out = append(out, WUValue{Variable: v.key, Label: v.label, Value: f, Unit: v.unit})
	}
	if len(out) == 0 {
		return nil, when, errors.New("la stazione non sta trasmettendo dati")
	}
	return out, when, nil
}
