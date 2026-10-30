package app

import (
	"context"
	"time"

	"github.com/fjzhangZzzzzz/subweft/internal/domain"
)

type PublishStatus struct {
	domain.Meta
	Attempt time.Time `json:"attempt"`
	Error   string    `json:"error,omitempty"`
}

// AutoPublish is retired: device updates do not mutate published definitions.
func (s *Service) AutoPublish(context.Context) {}
func sameResources(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
