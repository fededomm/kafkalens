package kafka

import (
	"context"
	"errors"

	"github.com/twmb/franz-go/pkg/kgo"

	"kafkalens/internal/store"
)

// tailSlots caps concurrent live tails: each one opens its own Kafka client.
var tailSlots = make(chan struct{}, 16)

var ErrTooManyTails = errors.New("too many live tails open")

// OpenTail takes a tail slot and creates a client positioned at the end of topic; no group is
// joined. stop closes the client and frees the slot.
func OpenTail(cl store.Cluster, topic string) (k *kgo.Client, stop func(), err error) {
	select {
	case tailSlots <- struct{}{}:
	default:
		return nil, nil, ErrTooManyTails
	}
	opts, err := ClientOpts(cl)
	if err == nil {
		k, err = kgo.NewClient(append(opts, kgo.ConsumeTopics(topic), kgo.ConsumeResetOffset(kgo.NewOffset().AtEnd()))...)
	}
	if err != nil {
		<-tailSlots
		return nil, nil, err
	}
	return k, func() { k.Close(); <-tailSlots }, nil
}

// PumpTail hands each polled batch of new records to emit until ctx ends (nil), emit fails
// or Kafka reports a fetch error (that error).
func PumpTail(ctx context.Context, k *kgo.Client, emit func([]Record) error) error {
	for {
		fs := k.PollFetches(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if err := fetchErr(fs); err != nil {
			return err
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
