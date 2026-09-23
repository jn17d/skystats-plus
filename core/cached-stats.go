package main

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

type CachedStatsService struct {
	pg                 *postgres
	mostSeenAircraftMu sync.Mutex
}

func NewCachedStatsService(pg *postgres) *CachedStatsService {
	return &CachedStatsService{pg: pg}
}

func (s *CachedStatsService) updateCachedStat(key string, value int) error {
	_, err := s.pg.db.Exec(context.Background(),
		`INSERT INTO cached_stats (stat_key, stat_value, last_updated)
		VALUES ($1, $2, NOW())
		ON CONFLICT (stat_key)
		DO UPDATE SET stat_value = $2, last_updated = NOW()`,
		key, value)
	return err
}

func (s *CachedStatsService) GetCachedTotalAircraft() (int, error) {
	var value int
	var lastUpdated time.Time
	err := s.pg.db.QueryRow(context.Background(),
		`SELECT stat_value, last_updated FROM cached_stats
		WHERE stat_key = 'total_aircraft'`).Scan(&value, &lastUpdated)

	if err == nil && time.Since(lastUpdated) < 24*time.Hour {
		log.Debug().
			Int("total_aircraft", value).
			Dur("cache_age", time.Since(lastUpdated)).
			Msg("Returning cached total_aircraft")
		return value, nil
	}

	log.Info().Msg("Cache stale or missing, recalculating total_aircraft...")

	var totalAircraft int
	err = s.pg.db.QueryRow(context.Background(),
		"SELECT COUNT(DISTINCT hex) FROM aircraft_data").Scan(&totalAircraft)
	if err != nil {
		log.Error().Err(err).Msg("Failed to count total aircraft")
		return 0, err
	}

	// Update cache
	err = s.updateCachedStat("total_aircraft", totalAircraft)
	if err != nil {
		log.Error().Err(err).Msg("Failed to update cached total_aircraft")
		return totalAircraft, nil
	}

	log.Info().
		Int("total_aircraft", totalAircraft).
		Msg("Successfully recalculated and cached total_aircraft")

	return totalAircraft, nil
}

const (
	// mostSeenAircraftCacheKey is the cached_stats_json key holding the leaderboard payload
	mostSeenAircraftCacheKey = "most_seen_aircraft_all"
	// mostSeenAircraftCacheTTL is how long a cached leaderboard is considered fresh
	mostSeenAircraftCacheTTL = 10 * time.Minute
	// mostSeenAircraftCacheSize is how many entries are cached. The configured
	// display limit is applied to the cached entries, so changing it does not
	// require a recalculation.
	mostSeenAircraftCacheSize = 100
)

// GetMostSeenAircraft returns the aircraft seen the most times, most seen first.
//
// The underlying aggregation scans the whole aircraft_data table, so the result
// is cached in the database (cached_stats_json) and expires after
// mostSeenAircraftCacheTTL.
func (s *CachedStatsService) GetMostSeenAircraft(limit int) ([]MostSeenAircraft, error) {

	aircraft, err := s.getMostSeenAircraft()
	if err != nil {
		return nil, err
	}

	if limit > 0 && limit < len(aircraft) {
		aircraft = aircraft[:limit]
	}

	return aircraft, nil
}

func (s *CachedStatsService) getMostSeenAircraft() ([]MostSeenAircraft, error) {

	if aircraft, cached := s.readMostSeenAircraftCache(); cached {
		return aircraft, nil
	}

	// Only recalculate once at a time, otherwise concurrent requests would all
	// run the (expensive) aggregation
	s.mostSeenAircraftMu.Lock()
	defer s.mostSeenAircraftMu.Unlock()

	// Another request may have refreshed the cache while we waited for the lock
	if aircraft, cached := s.readMostSeenAircraftCache(); cached {
		return aircraft, nil
	}

	log.Info().Msg("Leaderboard cache stale or missing, recalculating most seen aircraft...")

	start := time.Now()

	aircraft, err := s.calculateMostSeenAircraft()
	if err != nil {
		return nil, err
	}

	if err := s.cacheMostSeenAircraft(aircraft); err != nil {
		log.Error().Err(err).Msg("Failed to update cached most seen aircraft")
		return aircraft, nil
	}

	log.Info().
		Int("aircraft", len(aircraft)).
		Dur("took", time.Since(start)).
		Msg("Successfully recalculated and cached most seen aircraft")

	return aircraft, nil
}

// readMostSeenAircraftCache returns the cached leaderboard and whether it is still fresh
func (s *CachedStatsService) readMostSeenAircraftCache() ([]MostSeenAircraft, bool) {

	var statValue string
	var lastUpdated time.Time

	err := s.pg.db.QueryRow(context.Background(),
		`SELECT stat_value::text, last_updated FROM cached_stats_json
		WHERE stat_key = $1`, mostSeenAircraftCacheKey).Scan(&statValue, &lastUpdated)

	if err != nil {
		if err != pgx.ErrNoRows {
			log.Error().Err(err).Msg("Failed to read most seen aircraft cache")
		}
		return nil, false
	}

	if time.Since(lastUpdated) >= mostSeenAircraftCacheTTL {
		return nil, false
	}

	var aircraft []MostSeenAircraft
	if err := json.Unmarshal([]byte(statValue), &aircraft); err != nil {
		log.Error().Err(err).Msg("Failed to unmarshal cached most seen aircraft")
		return nil, false
	}

	log.Debug().
		Int("aircraft", len(aircraft)).
		Dur("cache_age", time.Since(lastUpdated)).
		Msg("Returning cached most seen aircraft")

	return aircraft, true
}

func (s *CachedStatsService) calculateMostSeenAircraft() ([]MostSeenAircraft, error) {

	// A sighting is a single visit, i.e. one row in aircraft_data, and a new row
	// is only inserted if the aircraft hasn't been seen for ~10 minutes.
	// Registration is taken from registration_data, falling back to the
	// registration received from readsb and finally the hex itself.
	query := `
		SELECT
			ad.hex,
			COALESCE(NULLIF(MAX(reg.registration), ''), NULLIF(MAX(ad.r), ''), UPPER(ad.hex)) AS registration,
			MAX(reg.type) AS type,
			MAX(reg.icao_type) AS icao_type,
			MAX(reg.registered_owner) AS operator,
			MAX(reg.registered_owner_country_name) AS country,
			COUNT(*) AS times_seen,
			COUNT(DISTINCT (ad.first_seen AT TIME ZONE 'UTC')::date) AS days_seen,
			MIN(ad.first_seen) AS first_seen,
			MAX(ad.last_seen) AS last_seen
		FROM aircraft_data ad
		LEFT JOIN registration_data reg ON reg.mode_s = ad.hex
		WHERE ad.hex <> ''
		GROUP BY ad.hex
		ORDER BY times_seen DESC, days_seen DESC, last_seen DESC
		LIMIT $1`

	rows, err := s.pg.db.Query(context.Background(), query, mostSeenAircraftCacheSize)
	if err != nil {
		log.Error().Err(err).Msg("Failed to query most seen aircraft")
		return nil, err
	}
	defer rows.Close()

	var aircraft []MostSeenAircraft

	for rows.Next() {
		var entry MostSeenAircraft

		err := rows.Scan(
			&entry.Hex,
			&entry.Registration,
			&entry.Type,
			&entry.IcaoType,
			&entry.Operator,
			&entry.Country,
			&entry.TimesSeen,
			&entry.DaysSeen,
			&entry.FirstSeen,
			&entry.LastSeen,
		)

		if err != nil {
			log.Error().Err(err).Msg("Failed to scan most seen aircraft row")
			continue
		}

		aircraft = append(aircraft, entry)
	}

	return aircraft, nil
}

func (s *CachedStatsService) cacheMostSeenAircraft(aircraft []MostSeenAircraft) error {

	statValue, err := json.Marshal(aircraft)
	if err != nil {
		return err
	}

	_, err = s.pg.db.Exec(context.Background(),
		`INSERT INTO cached_stats_json (stat_key, stat_value, last_updated)
		VALUES ($1, $2::jsonb, NOW())
		ON CONFLICT (stat_key)
		DO UPDATE SET stat_value = EXCLUDED.stat_value, last_updated = NOW()`,
		mostSeenAircraftCacheKey, string(statValue))

	return err
}
