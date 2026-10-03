package platform

// APIError carries a sanitized upstream status; collection uses 404 to retain missing builds.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string { return e.Message }
