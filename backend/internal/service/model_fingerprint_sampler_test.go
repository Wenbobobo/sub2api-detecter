package service

import (
	"strings"
	"testing"
)

func TestParseFingerprintResponseResponsesJSON(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6-sol","output":[{"type":"message","content":[{"type":"output_text","text":"100 200 300"}]}]}`)
	result := parseFingerprintResponse(DetectorAPIResponses, true, 200, body)
	if result.State != fingerprintSampleComplete {
		t.Fatalf("state = %s, error = %s", result.State, result.Error)
	}
	if result.Text != "100 200 300" || result.Numbers != 3 {
		t.Fatalf("text=%q numbers=%d", result.Text, result.Numbers)
	}
	if result.ResponseModel != "gpt-5.6-sol" {
		t.Fatalf("response model = %q", result.ResponseModel)
	}
}

func TestParseFingerprintResponseResponsesSSE(t *testing.T) {
	body := []byte(strings.Join([]string{
		`data: {"type":"response.created","response":{"model":"claude-sonnet-4.6"}}`,
		"",
		`data: {"type":"response.output_text.delta","delta":"1 2 3 "}`,
		"",
		`data: {"type":"response.output_text.delta","delta":"4 5 6"}`,
		"",
		`data: {"type":"response.completed"}`,
		"",
		"data: [DONE]",
		"",
	}, "\n"))
	result := parseFingerprintResponse(DetectorAPIResponses, false, 200, body)
	if result.State != fingerprintSampleComplete {
		t.Fatalf("state = %s, error = %s", result.State, result.Error)
	}
	if !strings.Contains(result.Text, "1 2 3 4 5 6") {
		t.Fatalf("text = %q", result.Text)
	}
	if result.ResponseModel != "claude-sonnet-4.6" {
		t.Fatalf("response model = %q", result.ResponseModel)
	}
}

func TestParseFingerprintResponseChatCompletions(t *testing.T) {
	stream := []byte(strings.Join([]string{
		`data: {"model":"gpt-5.1","choices":[{"delta":{"content":"10 20 "}}]}`,
		"",
		`data: {"choices":[{"delta":{"content":"30 40"}}]}`,
		"",
		"data: [DONE]",
		"",
	}, "\n"))
	result := parseFingerprintResponse(DetectorAPIChatCompletions, false, 200, stream)
	if result.Numbers != 4 {
		t.Fatalf("numbers = %d, text=%q, error=%q", result.Numbers, result.Text, result.Error)
	}
	if result.ResponseModel != "gpt-5.1" {
		t.Fatalf("response model = %q", result.ResponseModel)
	}

	jsonBody := []byte(`{"model":"gpt-5.4","choices":[{"message":{"content":[{"type":"text","text":"9 8 7"}]}}]}`)
	result = parseFingerprintResponse(DetectorAPIChatCompletions, true, 200, jsonBody)
	if result.Numbers != 3 || result.Text != "9 8 7" {
		t.Fatalf("json parse text=%q numbers=%d", result.Text, result.Numbers)
	}
}

func TestParseFingerprintResponseMessages(t *testing.T) {
	stream := []byte(strings.Join([]string{
		`data: {"type":"message_start","message":{"model":"claude-opus-4.6"}}`,
		"",
		`data: {"type":"content_block_delta","delta":{"text":"7 8 9"}}`,
		"",
		`data: {"type":"message_stop"}`,
		"",
	}, "\n"))
	result := parseFingerprintResponse(DetectorAPIMessages, false, 200, stream)
	if result.Numbers != 3 || result.ResponseModel != "claude-opus-4.6" {
		t.Fatalf("numbers=%d model=%q", result.Numbers, result.ResponseModel)
	}

	jsonBody := []byte(`{"model":"claude-sonnet-4.6","content":[{"type":"text","text":"5 6 7"}]}`)
	result = parseFingerprintResponse(DetectorAPIMessages, true, 200, jsonBody)
	if result.Numbers != 3 || result.Text != "5 6 7" {
		t.Fatalf("json parse text=%q numbers=%d", result.Text, result.Numbers)
	}
}

func TestParseFingerprintResponseErrors(t *testing.T) {
	result := parseFingerprintResponse(DetectorAPIResponses, false, 500, []byte("upstream exploded"))
	if result.State != fingerprintSampleFailed || !strings.Contains(result.Error, "500") {
		t.Fatalf("unexpected result: %+v", result)
	}

	// SSE error event after some text keeps the partial text but records the error.
	body := []byte(strings.Join([]string{
		`data: {"type":"response.output_text.delta","delta":"1 2 3"}`,
		"",
		`data: {"type":"error","error":{"message":"quota exceeded"}}`,
		"",
	}, "\n"))
	result = parseFingerprintResponse(DetectorAPIResponses, false, 200, body)
	if result.State != fingerprintSampleComplete || !strings.Contains(result.Error, "quota exceeded") {
		t.Fatalf("unexpected partial result: %+v", result)
	}

	// A non-stream request answered with an SSE body (relay ignored stream flag).
	body = []byte(strings.Join([]string{
		`data: {"type":"response.output_text.delta","delta":"11 22 33"}`,
		"",
		`data: {"type":"response.completed"}`,
		"",
	}, "\n"))
	result = parseFingerprintResponse(DetectorAPIResponses, true, 200, body)
	if result.Numbers != 3 {
		t.Fatalf("fallback SSE parse numbers = %d (%q)", result.Numbers, result.Text)
	}
}
