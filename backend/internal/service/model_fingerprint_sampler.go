package service

import (
	"encoding/json"
	"strings"

	lmfpd "github.com/Ikaleio/lm-detector/sdk/go"
)

// fingerprintSampleState values stored for one sample of one detection round.
const (
	fingerprintSampleComplete  = "complete"
	fingerprintSampleTruncated = "truncated"
	fingerprintSampleFailed    = "failed"
)

// fingerprintSampleResult is the outcome of one challenge request.
type fingerprintSampleResult struct {
	State         string
	Text          string
	Numbers       int
	ResponseModel string
	Error         string
	HTTPStatus    int
}

// fingerprintAPIDecision documents the protocol used for the request; it is
// the resolved value of DetectorConfig.API (never "auto").
func normalizeFingerprintAPI(api string) string {
	switch strings.ToLower(strings.TrimSpace(api)) {
	case DetectorAPIResponses:
		return DetectorAPIResponses
	case DetectorAPIChatCompletions:
		return DetectorAPIChatCompletions
	case DetectorAPIMessages:
		return DetectorAPIMessages
	default:
		return DetectorAPIResponses
	}
}

// parseFingerprintResponse extracts the model text from an upstream body.
// noStream selects the JSON decoder; streaming responses accept a JSON body
// too because some relays ignore stream=true.
func parseFingerprintResponse(api string, noStream bool, status int, body []byte) fingerprintSampleResult {
	out := fingerprintSampleResult{HTTPStatus: status, State: fingerprintSampleFailed}
	if status < 200 || status >= 300 {
		snippet := strings.TrimSpace(string(body))
		if len(snippet) > 500 {
			snippet = snippet[:500] + "..."
		}
		out.Error = "上游返回 HTTP " + itoa(status) + ": " + snippet
		return out
	}

	var text, responseModel, parseErr string
	if noStream {
		text, responseModel, parseErr = extractFingerprintJSONText(api, body)
		if strings.TrimSpace(text) == "" {
			// Relay ignored stream=false and answered with an SSE body.
			if sseText, sseModel, sseErr := extractFingerprintSSEText(api, body); strings.TrimSpace(sseText) != "" {
				text, responseModel, parseErr = sseText, sseModel, sseErr
			}
		}
	} else {
		text, responseModel, parseErr = extractFingerprintSSEText(api, body)
		if strings.TrimSpace(text) == "" {
			// Relay ignored stream=true and answered with a plain JSON body.
			if jsonText, jsonModel, jsonErr := extractFingerprintJSONText(api, body); strings.TrimSpace(jsonText) != "" {
				text, responseModel, parseErr = jsonText, jsonModel, jsonErr
			}
		}
	}
	if responseModel == "" {
		responseModel = extractFingerprintBodyModel(api, body)
	}

	out.Text = text
	out.ResponseModel = responseModel
	out.Numbers = len(lmfpd.ParseNumbers(text))
	if out.Numbers == 0 {
		if parseErr != "" {
			out.Error = parseErr
		} else {
			out.Error = "上游响应没有可用的数字文本"
		}
		return out
	}
	out.State = fingerprintSampleComplete
	if parseErr != "" {
		// Keep the partial text for scoring; the error is diagnostic only.
		out.Error = parseErr
	}
	return out
}

// extractFingerprintSSEText collects streamed answer text for one protocol.
func extractFingerprintSSEText(api string, body []byte) (string, string, string) {
	var builder strings.Builder
	responseModel := ""
	api = normalizeFingerprintAPI(api)
	for _, rawLine := range strings.Split(string(body), "\n") {
		line := strings.TrimSpace(rawLine)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			break
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}
		if responseModel == "" {
			responseModel = fingerprintFirstNonEmptyString(event["model"], nestedString(event, "response", "model"), nestedString(event, "message", "model"))
		}
		switch api {
		case DetectorAPIMessages:
			eventType := fingerprintAsString(event["type"])
			switch eventType {
			case "content_block_delta":
				if delta, ok := event["delta"].(map[string]any); ok {
					builder.WriteString(fingerprintAsString(delta["text"]))
				}
			case "message_stop":
				return builder.String(), responseModel, ""
			case "error":
				return builder.String(), responseModel, fingerprintEventError(event)
			}
		case DetectorAPIChatCompletions:
			if event["error"] != nil {
				return builder.String(), responseModel, fingerprintEventError(event)
			}
			for _, choice := range asAnySlice(event["choices"]) {
				choiceMap, ok := choice.(map[string]any)
				if !ok {
					continue
				}
				if delta, ok := choiceMap["delta"].(map[string]any); ok {
					builder.WriteString(contentString(delta["content"]))
				}
			}
		default:
			eventType := fingerprintAsString(event["type"])
			switch eventType {
			case "response.output_text.delta":
				builder.WriteString(fingerprintAsString(event["delta"]))
			case "response.completed", "response.done":
				return builder.String(), responseModel, ""
			case "response.failed", "error":
				return builder.String(), responseModel, fingerprintEventError(event)
			}
		}
	}
	return builder.String(), responseModel, ""
}

// extractFingerprintJSONText extracts the answer text from a JSON completion.
func extractFingerprintJSONText(api string, body []byte) (string, string, string) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		snippet := strings.TrimSpace(string(body))
		if len(snippet) > 300 {
			snippet = snippet[:300] + "..."
		}
		return "", "", "上游返回了无法解析的响应: " + snippet
	}
	responseModel := fingerprintAsString(payload["model"])
	if payload["error"] != nil {
		return "", responseModel, fingerprintEventError(payload)
	}
	api = normalizeFingerprintAPI(api)
	var text string
	switch api {
	case DetectorAPIMessages:
		for _, part := range asAnySlice(payload["content"]) {
			partMap, ok := part.(map[string]any)
			if !ok || fingerprintAsString(partMap["type"]) != "text" {
				continue
			}
			text += fingerprintAsString(partMap["text"])
		}
	case DetectorAPIChatCompletions:
		for _, choice := range asAnySlice(payload["choices"]) {
			choiceMap, ok := choice.(map[string]any)
			if !ok {
				continue
			}
			if message, ok := choiceMap["message"].(map[string]any); ok {
				text += contentString(message["content"])
			}
		}
	default:
		text = fingerprintAsString(payload["output_text"])
		if text == "" {
			for _, item := range asAnySlice(payload["output"]) {
				itemMap, ok := item.(map[string]any)
				if !ok || fingerprintAsString(itemMap["type"]) != "message" {
					continue
				}
				for _, part := range asAnySlice(itemMap["content"]) {
					partMap, ok := part.(map[string]any)
					if !ok || fingerprintAsString(partMap["type"]) != "output_text" {
						continue
					}
					text += fingerprintAsString(partMap["text"])
				}
			}
		}
	}
	return text, responseModel, ""
}

// extractFingerprintBodyModel best-effort extracts the upstream model name from
// a JSON body that failed to parse through the primary decoders.
func extractFingerprintBodyModel(api string, body []byte) string {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	if model := fingerprintAsString(payload["model"]); model != "" {
		return model
	}
	return nestedString(payload, "response", "model")
}

// fingerprintEventError renders a protocol error payload into one line.
func fingerprintEventError(event map[string]any) string {
	if errObj, ok := event["error"].(map[string]any); ok {
		message := fingerprintFirstNonEmptyString(errObj["message"], errObj["type"], errObj["code"])
		if message != "" {
			return message
		}
	}
	if response, ok := event["response"].(map[string]any); ok {
		if errObj, ok := response["error"].(map[string]any); ok {
			if message := fingerprintAsString(errObj["message"]); message != "" {
				return message
			}
		}
	}
	if message := fingerprintAsString(event["message"]); message != "" {
		return message
	}
	return "上游返回错误事件"
}

func nestedString(payload map[string]any, keys ...string) string {
	var current any = payload
	for _, key := range keys {
		currentMap, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = currentMap[key]
	}
	return fingerprintAsString(current)
}

func fingerprintAsString(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}

func asAnySlice(value any) []any {
	if list, ok := value.([]any); ok {
		return list
	}
	return nil
}

func fingerprintFirstNonEmptyString(values ...any) string {
	for _, value := range values {
		if s := fingerprintAsString(value); s != "" {
			return s
		}
	}
	return ""
}

// contentString reads either a plain string content or an array of text parts.
func contentString(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	var builder strings.Builder
	for _, part := range asAnySlice(value) {
		partMap, ok := part.(map[string]any)
		if !ok {
			continue
		}
		if text := fingerprintAsString(partMap["text"]); text != "" {
			builder.WriteString(text)
		}
	}
	return builder.String()
}

// itoa is a tiny helper so this file does not import strconv only for errors.
func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	var digits [20]byte
	position := len(digits)
	for value > 0 {
		position--
		digits[position] = byte('0' + value%10)
		value /= 10
	}
	if negative {
		position--
		digits[position] = '-'
	}
	return string(digits[position:])
}
