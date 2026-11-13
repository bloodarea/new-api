package helper

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func systemOneTestContext(body string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/systemone", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c
}

// The decision payload must reach the upstream byte for byte: questions and
// fields the host does not understand cannot survive a typed conversion, which
// is why the request keeps the client's JSON.
func TestSystemOneRequestKeepsRawBody(t *testing.T) {
	body := `{"model":"jev-latest","state":"判断这段文本","questions":{"intent":{"type":"choice","options":["a","b"]}},"future_field":{"nested":true}}`

	request, err := GetAndValidateSystemOneRequest(systemOneTestContext(body))
	require.NoError(t, err)
	assert.Equal(t, "jev-latest", request.Model)
	assert.Equal(t, body, string(request.RawBody))
	// The pre-consume estimate counts the judged text, not the question schema.
	assert.Equal(t, "判断这段文本", request.GetTokenCountMeta().CombineText)
}

// Channel selection happens before the request reaches the upstream, so a body
// without a model is rejected locally instead of failing channel lookup.
func TestSystemOneRequestRequiresModel(t *testing.T) {
	_, err := GetAndValidateSystemOneRequest(systemOneTestContext(`{"state":"判断这段文本","questions":{}}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "model is required")
}
