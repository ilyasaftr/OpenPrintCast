package net

const maxPendingDocumentBytes = 256 << 20

// documentBudget accounts for the document bytes held in memory, both
// uploaded documents still awaiting their job to close (pending) and
// documents handed to the spooler (processing). It has no lock of its
// own: it is guarded by the mutex of the store that embeds it.
type documentBudget struct {
	pendingBytes    int
	processingBytes int
}

// fits reports whether size more bytes stay within the in-flight limit.
func (b *documentBudget) fits(size int) bool {
	return size <= maxPendingDocumentBytes-b.pendingBytes-b.processingBytes
}

func (b *documentBudget) addPending(size int)        { b.pendingBytes += size }
func (b *documentBudget) releasePending(size int)    { b.pendingBytes -= size }
func (b *documentBudget) addProcessing(size int)     { b.processingBytes += size }
func (b *documentBudget) releaseProcessing(size int) { b.processingBytes -= size }
