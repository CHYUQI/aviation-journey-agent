package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// flightNumberMatch 是 EOOB 航班号搜索接口返回的一条航段记录。
type flightNumberMatch struct {
	Number            string
	IATAFrom          string
	IATATo            string
	NextDepartureDate string
}

// fetchFlightNumberMatches 向 EOOB 航班号搜索接口查一次，返回全部匹配航段。
//
// 注意：同一航班号可能对应多条航段（经停航班会被拆成 TSN→YIW、YIW→SWA），
// 调用方不能假设只有一条。
func fetchFlightNumberMatches(ctx context.Context, client *http.Client, number string) ([]flightNumberMatch, error) {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}

	endpoint, err := url.Parse(eoobFlightNumberSearchURL)
	if err != nil {
		return nil, err
	}
	params := endpoint.Query()
	params.Set("s", strings.ToUpper(strings.TrimSpace(number)))
	params.Set("lang", "zh")
	endpoint.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	setEOOBHeaders(req, "application/json", eoobJSONRequest)
	req.Header.Set("Origin", "https://www.eoob.com.cn")
	req.Header.Set("Referer", "https://www.eoob.com.cn/hangban-zhuizong")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, eoobSearchMaxBytes))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var payload struct {
		Matches []struct {
			FlightNumber      string `json:"flightnumber"`
			IATAFrom          string `json:"iata_from"`
			IATATo            string `json:"iata_to"`
			NextDepartureDate string `json:"next_departure_date"`
		} `json:"matches"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("解析搜索结果失败: %w", err)
	}

	matches := make([]flightNumberMatch, 0, len(payload.Matches))
	for _, m := range payload.Matches {
		matches = append(matches, flightNumberMatch{
			Number:            strings.ToUpper(strings.TrimSpace(m.FlightNumber)),
			IATAFrom:          strings.ToUpper(strings.TrimSpace(m.IATAFrom)),
			IATATo:            strings.ToUpper(strings.TrimSpace(m.IATATo)),
			NextDepartureDate: strings.TrimSpace(m.NextDepartureDate),
		})
	}
	return matches, nil
}

// lookupFlightSegments 返回某个航班号在 EOOB 里登记的全部航段（去重、按出发机场排序）。
func lookupFlightSegments(ctx context.Context, number string) ([]ResolvedFlightIdentity, error) {
	wanted := strings.ToUpper(strings.TrimSpace(number))
	matches, err := fetchFlightNumberMatches(ctx, nil, wanted)
	if err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	segments := make([]ResolvedFlightIdentity, 0, len(matches))
	for _, m := range matches {
		if m.Number != wanted || !validAirportIATA(m.IATAFrom) || !validAirportIATA(m.IATATo) {
			continue
		}
		key := m.IATAFrom + "-" + m.IATATo
		if seen[key] {
			continue
		}
		seen[key] = true
		segments = append(segments, ResolvedFlightIdentity{
			Number: wanted, Date: m.NextDepartureDate, From: m.IATAFrom, To: m.IATATo,
		})
	}
	if len(segments) == 0 {
		return nil, fmt.Errorf("EOOB 没有 %s 的航段记录", wanted)
	}
	sort.Slice(segments, func(i, j int) bool {
		if segments[i].From != segments[j].From {
			return segments[i].From < segments[j].From
		}
		return segments[i].To < segments[j].To
	})
	return segments, nil
}
