package skill

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	eoobFlightNumberSearchURL = "https://apil.eoob.com/api/flightnumbers/search.json"
	eoobResolutionCacheTTL    = 5 * time.Minute
	eoobSearchMaxBytes        = 1 << 20
)

type cachedEOOBIdentity struct {
	identity ResolvedFlightIdentity
	expires  time.Time
}

type cachedEOOBString struct {
	value   string
	expires time.Time
}

var (
	eoobFlightIdentityCache  sync.Map
	eoobAirportTimezoneCache sync.Map
)

func cachedEOOBFlightIdentity(number string) (ResolvedFlightIdentity, bool) {
	value, ok := eoobFlightIdentityCache.Load(strings.ToUpper(strings.TrimSpace(number)))
	if !ok {
		return ResolvedFlightIdentity{}, false
	}
	cached, ok := value.(cachedEOOBIdentity)
	if !ok || time.Now().After(cached.expires) {
		eoobFlightIdentityCache.Delete(strings.ToUpper(strings.TrimSpace(number)))
		return ResolvedFlightIdentity{}, false
	}
	return cached.identity, true
}

func cacheEOOBFlightIdentity(identity ResolvedFlightIdentity) {
	eoobFlightIdentityCache.Store(strings.ToUpper(strings.TrimSpace(identity.Number)), cachedEOOBIdentity{
		identity: identity,
		expires:  time.Now().Add(eoobResolutionCacheTTL),
	})
}

func cachedEOOBAirportTimezone(iata string) (string, bool) {
	value, ok := eoobAirportTimezoneCache.Load(strings.ToUpper(strings.TrimSpace(iata)))
	if !ok {
		return "", false
	}
	cached, ok := value.(cachedEOOBString)
	if !ok || time.Now().After(cached.expires) {
		eoobAirportTimezoneCache.Delete(strings.ToUpper(strings.TrimSpace(iata)))
		return "", false
	}
	return cached.value, true
}

func cacheEOOBAirportTimezone(iata, timezone string) {
	eoobAirportTimezoneCache.Store(strings.ToUpper(strings.TrimSpace(iata)), cachedEOOBString{
		value:   timezone,
		expires: time.Now().Add(eoobResolutionCacheTTL),
	})
}

// FlightIdentitySkill 通过 EOOB 页面自己的航班号搜索接口解析最近班次。
//
// 输入只有航班号。接口返回航线、最近出发日期，和页面上搜索框使用的是同一条路径。
type FlightIdentitySkill struct {
	Client *http.Client
}

func NewFlightIdentitySkill() FlightIdentitySkill {
	return FlightIdentitySkill{Client: &http.Client{Timeout: 15 * time.Second}}
}

func (FlightIdentitySkill) Name() string { return "flight_identity" }

func (FlightIdentitySkill) Supports(field string) bool {
	return field == FieldFlightIdentity
}

func (s FlightIdentitySkill) Execute(ctx context.Context, query Query) (Result, error) {
	if len(query.Journey.Flights) == 0 {
		return Result{Issues: []string{"行程里没有航段，无法解析航班"}}, nil
	}

	number := strings.ToUpper(strings.TrimSpace(query.Journey.Flights[0].Number))
	if number == "" {
		return Result{Issues: []string{"行程里没有航班号，无法解析最近班次"}}, nil
	}
	if identity, ok := cachedEOOBFlightIdentity(number); ok {
		return flightIdentityResult(identity), nil
	}

	identity, err := s.searchFlightNumber(ctx, number)
	if err != nil {
		return Result{Issues: []string{"EOOB 航班号搜索失败：" + err.Error()}}, nil
	}
	cacheEOOBFlightIdentity(identity)
	return flightIdentityResult(identity), nil
}

func flightIdentityResult(identity ResolvedFlightIdentity) Result {
	return Result{Observations: []Observation{{
		Field:      FieldFlightIdentity,
		Value:      identity,
		Source:     "EOOB 航班号搜索接口",
		ObservedAt: time.Now().Format(time.RFC3339),
		Confidence: "high",
	}}}
}

func (s FlightIdentitySkill) searchFlightNumber(ctx context.Context, number string) (ResolvedFlightIdentity, error) {
	matches, err := fetchFlightNumberMatches(ctx, s.Client, number)
	if err != nil {
		return ResolvedFlightIdentity{}, err
	}

	// 同一个航班号可能对应多条航段（去程/回程/经停段），这里按航段去重。
	// 只有唯一航段时自动补全；多条候选一律不猜，交给调用方提供 date/from/to。
	candidates := map[string]ResolvedFlightIdentity{}
	for _, m := range matches {
		if m.Number != strings.ToUpper(strings.TrimSpace(number)) {
			continue
		}
		if m.NextDepartureDate == "" || !validAirportIATA(m.IATAFrom) || !validAirportIATA(m.IATATo) {
			continue
		}

		key := m.IATAFrom + "-" + m.IATATo
		existing, ok := candidates[key]
		if !ok || m.NextDepartureDate < existing.Date {
			candidates[key] = ResolvedFlightIdentity{
				Number: m.Number, Date: m.NextDepartureDate, From: m.IATAFrom, To: m.IATATo,
			}
		}
	}

	switch len(candidates) {
	case 0:
		return ResolvedFlightIdentity{}, errors.New("搜索结果里没有匹配的航班号")
	case 1:
		for _, identity := range candidates {
			return identity, nil
		}
	}

	options := make([]string, 0, len(candidates))
	for _, identity := range candidates {
		options = append(options, fmt.Sprintf("%s %s→%s", identity.Date, identity.From, identity.To))
	}
	sort.Strings(options)
	return ResolvedFlightIdentity{}, fmt.Errorf(
		"航班号 %s 对应多条航线：%s；无法自动确认，请提供 date/from/to", number, strings.Join(options, "；"))
}
