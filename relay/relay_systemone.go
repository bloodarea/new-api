package relay

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relay/requestplugin"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/MoeForge/moego/ojson"
	"github.com/gin-gonic/gin"
)

// SystemOneHelper relays a TypeSafe Jev decision request. The protocol has no
// chat shape at all: state and questions travel upstream unchanged and the
// answers come back exactly as the decision model wrote them, so nothing is
// converted in either direction. The response's usage block is read only to
// settle the charge.
func SystemOneHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {
	info.InitChannelMeta(c)

	request, ok := info.Request.(*dto.SystemOneRequest)
	if !ok {
		return types.NewErrorWithStatusCode(fmt.Errorf("invalid request type, expected *dto.SystemOneRequest, got %T", info.Request), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}

	if err := helper.ModelMappedHelper(c, info, request); err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}

	jsonData := request.RawBody
	if info.UpstreamModelName != "" && info.UpstreamModelName != info.OriginModelName {
		// Channel model mapping rewrites the one field the upstream routes on;
		// every other member of the decision request stays byte-identical.
		doc := ojson.NewDoc(request.RawBody)
		if err := relaycommon.SetJSONMember(doc, "", "model", info.UpstreamModelName); err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		jsonData = doc.Bytes()
	}

	if jsonData, newAPIError = requestplugin.Outbound(c, info, jsonData); newAPIError != nil {
		return newAPIError
	}
	if len(info.ParamOverride) > 0 {
		var err error
		jsonData, err = relaycommon.ApplyParamOverrideWithRelayInfo(jsonData, info)
		if err != nil {
			return newAPIErrorFromParamOverride(err)
		}
	}
	if newAPIError = requestplugin.Egress(c, info, jsonData); newAPIError != nil {
		return newAPIError
	}

	logger.LogDebug(c, "systemone request body: %s", jsonData)
	body, closer, err := relaycommon.NewOutboundJSONBody(jsonData)
	if err != nil {
		return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	defer closer.Close()

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)

	resp, err := adaptor.DoRequest(c, info, body)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}

	statusCodeMappingStr := c.GetString("status_code_mapping")
	httpResp, ok := resp.(*http.Response)
	if !ok || httpResp == nil {
		return types.NewOpenAIError(errors.New("invalid http response"), types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		// The decision model's own rejection — an unknown question type, a
		// missing criteria map, a quota stop — is what the caller has to act
		// on. Its body is not an OpenAI error envelope, so RelayErrorHandler
		// would reduce it to a bare status code; carry the body itself as the
		// message and keep the upstream status.
		upstreamBody, readErr := io.ReadAll(httpResp.Body)
		service.CloseResponseBodyGracefully(httpResp)
		if readErr != nil {
			return types.NewError(readErr, types.ErrorCodeReadResponseBodyFailed, types.ErrOptionWithSkipRetry())
		}
		detail := strings.TrimSpace(string(upstreamBody))
		if detail == "" {
			detail = fmt.Sprintf("upstream rejected the decision request with status %d", httpResp.StatusCode)
		}
		logger.LogError(c, common.LogText("systemone upstream rejected the request: %s", common.LocalLogPreview(detail)))
		newAPIError = types.NewOpenAIError(errors.New(detail), types.ErrorCodeBadResponseStatusCode, httpResp.StatusCode)
		service.ResetStatusCode(newAPIError, statusCodeMappingStr)
		return newAPIError
	}

	responseBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return types.NewError(err, types.ErrorCodeReadResponseBodyFailed, types.ErrOptionWithSkipRetry())
	}

	// The decision payload — answers, probabilities, legend — reaches the
	// client as the upstream wrote it; only the usage block below is read.
	if contentType := httpResp.Header.Get("Content-Type"); contentType != "" {
		c.Writer.Header().Set("Content-Type", contentType)
	}
	c.Writer.WriteHeader(httpResp.StatusCode)
	if _, err := c.Writer.Write(responseBody); err != nil {
		return types.NewError(err, types.ErrorCodeBadResponse, types.ErrOptionWithSkipRetry())
	}

	// Jev bills input tokens only: the answers are free. A response without a
	// readable usage block settles nothing here and leaves the request to the
	// host's own estimate; a negative count never becomes a negative charge.
	var usage *dto.Usage
	var parsed dto.SystemOneResponse
	if err := common.Unmarshal(responseBody, &parsed); err != nil {
		logger.LogWarn(c, common.LogText("systemone: unreadable upstream response, no usage to bill: %s", common.LocalLogPreview(err.Error())))
	} else if parsed.Usage != nil {
		inputTokens := max(parsed.Usage.InputTokens, 0)
		if inputTokens != parsed.Usage.InputTokens {
			logger.LogWarn(c, common.LogText("systemone: upstream reported %d input tokens, billing zero", parsed.Usage.InputTokens))
		}
		usage = &dto.Usage{
			PromptTokens: inputTokens,
			TotalTokens:  inputTokens,
		}
	}
	service.PostTextConsumeQuota(c, info, usage, nil)
	return nil
}
