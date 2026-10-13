package platform

// Persistence records shared by the application and storage adapter.
type SavedConnection struct {
	Connection Connection
	SecretRef  string
}
type HistoryFilter struct {
	ConnectionID string `json:"connectionId"`
	Job          string `json:"job"`
	Status       string `json:"status"`
	Since        int64  `json:"since"`
	Until        int64  `json:"until"`
	Offset       int    `json:"offset"`
}
type HistoryPage struct {
	Builds []Build `json:"builds"`
	Total  int     `json:"total"`
	Size   int64   `json:"size"`
}
