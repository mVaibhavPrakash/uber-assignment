package seeding

import (
	"context"
	"fmt"
)

// Reads prepared JSON records and sends bounded batches to storage.
func streamObservationBatches(ctx context.Context, writer seedWriter, path string) (Summary, error) {
	batch := make([]Observation, 0, 10000)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := writer.CopyObservations(ctx, batch); err != nil {
			return err
		}
		batch = batch[:0]
		return nil
	}
	summary, err := StreamSeedObservations(ctx, path, func(row Observation) error {
		batch = append(batch, row)
		if len(batch) == cap(batch) {
			return flush()
		}
		return nil
	})
	if err != nil {
		return summary, fmt.Errorf("read seed JSON: %w", err)
	}
	return summary, flush()
}
