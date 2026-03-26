package observation

import "testing"

func TestApplyActionHeuristicsPrefersFreshCaptureForObservation(t *testing.T) {
	service := &Service{}

	req := service.applyActionHeuristics(ChatRequest{
		Message: "请观察一下当前摄像头画面里有什么",
	})

	if !req.CaptureFresh {
		t.Fatal("expected fresh capture for live observation request")
	}
	if req.UseLatestImage {
		t.Fatal("did not expect latest-image reuse for live observation request")
	}
}

func TestApplyActionHeuristicsUsesLatestOnlyWhenExplicitlyRequested(t *testing.T) {
	service := &Service{}

	req := service.applyActionHeuristics(ChatRequest{
		Message: "请复用最新一帧图片回答",
	})

	if req.CaptureFresh {
		t.Fatal("did not expect fresh capture when latest image reuse is explicit")
	}
	if !req.UseLatestImage {
		t.Fatal("expected latest-image reuse when explicitly requested")
	}
}

func TestLooksLikeToolDeferral(t *testing.T) {
	if !looksLikeToolDeferral("我需要先调用 camera_read 才能回答这个问题。") {
		t.Fatal("expected Chinese tool deferral to be detected")
	}
	if !looksLikeToolDeferral("I need to call the camera tool before I can answer.") {
		t.Fatal("expected English tool deferral to be detected")
	}
	if looksLikeToolDeferral("画面右侧有一台白色履带机器人。") {
		t.Fatal("did not expect normal visual answer to be treated as tool deferral")
	}
}
