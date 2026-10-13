package application

import (
	"idp-dashboard/internal/collector"
	"idp-dashboard/internal/platform"
)

// Store is owned by the application; SQLite is one implementation.
type Store interface {
	collector.BuildArchive
	Connections() ([]platform.SavedConnection, error)
	SaveConnection(platform.Connection, string) error
	DeleteConnection(string) error
	History(platform.HistoryFilter) (platform.HistoryPage, error)
	Preference(string) (string, error)
	SavePreference(string, string) error
	Backup(string) error
	Close() error
}
type Credentials interface {
	Get(string) (string, error)
	Set(string, string) error
	Delete(string) error
}
