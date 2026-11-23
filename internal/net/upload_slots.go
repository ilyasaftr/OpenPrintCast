package net

import "context"

const maxConcurrentUploads = 2

// uploadSlots bounds how many request bodies are read at once.
type uploadSlots chan struct{}

func newUploadSlots(limit int) uploadSlots { return make(uploadSlots, limit) }

// acquire blocks for a free slot, giving up when ctx is done.
func (u uploadSlots) acquire(ctx context.Context) bool {
	select {
	case u <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	}
}

func (u uploadSlots) release() { <-u }
