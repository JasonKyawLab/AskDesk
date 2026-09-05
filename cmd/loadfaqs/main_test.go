package main

import (
	"errors"
	"testing"
)

func TestIsTransient(t *testing.T) {
	retry := []error{
		errors.New("gemini: embed status 503: The service is currently unavailable."),
		errors.New("gemini: status 429: rate limit"),
		errors.New("gemini: request failed: context deadline exceeded"),
		errors.New("model is overloaded"),
		errors.New("read: connection reset by peer"),
	}
	for _, err := range retry {
		if !isTransient(err) {
			t.Errorf("isTransient(%q) = false, want true", err)
		}
	}

	permanent := []error{
		errors.New("gemini: status 400: invalid request"),
		errors.New("gemini: status 401: bad api key"),
		errors.New("insert faq: duplicate key"),
	}
	for _, err := range permanent {
		if isTransient(err) {
			t.Errorf("isTransient(%q) = true, want false", err)
		}
	}
}
