package natsstream

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/nats-io/nats.go"
)

type manager interface {
	StreamInfo(string, ...nats.JSOpt) (*nats.StreamInfo, error)
	AddStream(*nats.StreamConfig, ...nats.JSOpt) (*nats.StreamInfo, error)
}

func Config(name, subject string) *nats.StreamConfig {
	return &nats.StreamConfig{
		Name:              name,
		Subjects:          []string{subject},
		Storage:           nats.FileStorage,
		Discard:           nats.DiscardOld,
		Retention:         nats.WorkQueuePolicy,
		MaxMsgs:           -1,
		MaxBytes:          -1,
		MaxAge:            30 * 24 * time.Hour,
		Replicas:          1,
		MaxMsgsPerSubject: -1,
	}
}

// Ensure creates the stream when absent and validates streams managed externally.
func Ensure(js manager, name, subject string) error {
	info, err := js.StreamInfo(name)
	if err == nil {
		return validate(info, subject)
	}
	if !errors.Is(err, nats.ErrStreamNotFound) {
		return fmt.Errorf("get stream %q: %w", name, err)
	}

	info, err = js.AddStream(Config(name, subject))
	if err != nil {
		// Another process may have created the stream between lookup and creation.
		info, lookupErr := js.StreamInfo(name)
		if lookupErr != nil {
			return fmt.Errorf("create stream %q: %w", name, err)
		}
		return validate(info, subject)
	}

	return validate(info, subject)
}

func validate(info *nats.StreamInfo, subject string) error {
	if !slices.Contains(info.Config.Subjects, subject) {
		return fmt.Errorf("stream %q does not include subject %q", info.Config.Name, subject)
	}
	if info.Config.Retention != nats.WorkQueuePolicy {
		return fmt.Errorf("stream %q must use work queue retention", info.Config.Name)
	}
	return nil
}
