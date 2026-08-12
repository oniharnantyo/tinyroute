package cloudcode

import "encoding/json"

// Envelope wraps a native Gemini request for the CloudCode API backend.
type Envelope struct {
	Project     string          `json:"project"`
	Model       string          `json:"model"`
	UserAgent   string          `json:"userAgent"`
	RequestType string          `json:"requestType"`
	RequestID   string          `json:"requestId"`
	Request     json.RawMessage `json:"request"`
}

// projectIDValue represents a project ID that may arrive either as a plain string
// or as a JSON object `{"id": "..."}`.
type projectIDValue string

func (p *projectIDValue) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*p = projectIDValue(s)
		return nil
	}
	var obj struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(b, &obj); err == nil && obj.ID != "" {
		*p = projectIDValue(obj.ID)
		return nil
	}
	return nil
}

func (p projectIDValue) String() string {
	return string(p)
}

// loadCodeAssistResponse represents the JSON response from loadCodeAssist.
type loadCodeAssistResponse struct {
	CloudAICompanionProject projectIDValue `json:"cloudaicompanionProject"`
}

// onboardUserRequest represents the JSON request payload for onboardUser.
type onboardUserRequest struct {
	TierID string `json:"tierId"`
}

// onboardUserResponse represents the JSON response from onboardUser.
type onboardUserResponse struct {
	CloudAICompanionProject projectIDValue `json:"cloudaicompanionProject"`
}
