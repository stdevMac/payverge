package server

// ErrorLogPayload represents the structure of the error log received from the frontend
type ErrorLogPayload struct {
	Timestamp      string                 `json:"timestamp"`
	Error          string                 `json:"error"`
	Component      string                 `json:"component"`
	Function       string                 `json:"function"`
	AdditionalInfo map[string]interface{} `json:"additionalInfo,omitempty"`
}
