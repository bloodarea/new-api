package dto

import (
	"encoding/json"
	"net/http"

	"github.com/QuantumNous/new-api/relaykit/types"
)

// SystemOneRequest is a TypeSafe Jev decision request. The protocol shares no
// structure with chat completions: "state" is the text under judgement and
// "questions" describes what to decide. RawBody keeps the client's JSON so
// every field — including ones the decision model gains later — reaches it
// unchanged.
type SystemOneRequest struct {
	Model   string          `json:"model"`
	State   string          `json:"state"`
	RawBody json.RawMessage `json:"-"`
}

// SystemOneResponse is the decision model's answer. Only the usage block is
// read here; "answers" and the rest of the payload reach the client as the
// upstream sent them.
type SystemOneResponse struct {
	Model string          `json:"model"`
	Usage *SystemOneUsage `json:"usage,omitempty"`
}

// SystemOneUsage is the decision model's own usage shape. Input tokens are the
// billed quantity; output tokens are reported for observability only.
type SystemOneUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

func (r *SystemOneRequest) IsStream(_ *http.Request) bool {
	return false
}

// GetTokenCountMeta estimates the request from the judged text only: the
// question definitions are a schema, not prompt content.
func (r *SystemOneRequest) GetTokenCountMeta() *types.TokenCountMeta {
	return &types.TokenCountMeta{
		CombineText: r.State,
		TokenType:   types.TokenTypeTokenizer,
	}
}

func (r *SystemOneRequest) SetModelName(modelName string) {
	if modelName != "" {
		r.Model = modelName
	}
}
