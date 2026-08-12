package openai

import (
	"encoding/json"
	"net/http"
)

// ModelsResponse is the OpenAI-format model listing.
type ModelsResponse struct {
	Object string       `json:"object"`
	Data   []ModelEntry `json:"data"`
}

// ModelEntry is one model in the list.
type ModelEntry struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// WriteModelsResponse writes a /v1/models response synthesized from the given model names.
func WriteModelsResponse(w http.ResponseWriter, models []string) {
	data := make([]ModelEntry, 0, len(models))
	for _, m := range models {
		data = append(data, ModelEntry{
			ID:      m,
			Object:  "model",
			Created: 0,
			OwnedBy: "tinyroute",
		})
	}
	resp := ModelsResponse{
		Object: "list",
		Data:   data,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
