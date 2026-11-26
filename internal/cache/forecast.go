package cache

import (
	"mobilitycontrolroom/internal/repository"
	"sync/atomic"
)

type ForecastStore struct {
	snapshot atomic.Pointer[repository.ForecastSnapshot]
}

func cloneForecast(s repository.ForecastSnapshot) repository.ForecastSnapshot {
	s.Hours = append([]repository.ForecastHour(nil), s.Hours...)
	return s
}

func (s *ForecastStore) Set(snapshot repository.ForecastSnapshot) {
	copiedData := cloneForecast(snapshot)
	s.snapshot.Store(&copiedData)
}

func (s *ForecastStore) Get() (repository.ForecastSnapshot, bool) {
	value := s.snapshot.Load()
	if value == nil {
		return repository.ForecastSnapshot{}, false
	}
	return cloneForecast(*value), true
}

func (s *ForecastStore) Clear() {
	s.snapshot.Store(nil)
}
