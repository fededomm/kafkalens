package kafka

import (
	"context"

	"github.com/twmb/franz-go/pkg/kgo"

	"kafkalens/internal/store"
)

// tailSlots caps concurrent live tails: each one opens its own Kafka client.
var tailSlots = make(chan struct{}, 16)

func AcquireTail() bool {
	select {
	case tailSlots <- struct{}{}:
		return true
	default:
		return false
	}
}

func ReleaseTail() { <-tailSlots }

// OpenTail creates a client positioned at the end of topic; no group is joined.
func OpenTail(cl store.Cluster, topic string) (*kgo.Client, error) {
	opts, err := ClientOpts(cl)
	if err != nil {
		return nil, err
	}
	return kgo.NewClient(append(opts, kgo.ConsumeTopics(topic), kgo.ConsumeResetOffset(kgo.NewOffset().AtEnd()))...)
}

// PumpTail hands each polled batch of new records to emit until ctx ends or emit fails.
func PumpTail(ctx context.Context, k *kgo.Client, emit func([]Record) error) error {
	for {
		fs := k.PollFetches(ctx)
		if ctx.Err() != nil {
			return nil
		}
		var batch []Record
		fs.EachRecord(func(rec *kgo.Record) { batch = append(batch, toRecord(rec)) })
		if len(batch) == 0 {
			continue
		}
		if err := emit(batch); err != nil {
			return err
		}
	}
}
