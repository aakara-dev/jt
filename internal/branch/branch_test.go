package branch

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestName(t *testing.T) {
	assert.Equal(t, "feat/DATA-12-add-retry-to-the-loader", Name("data-12", "Add retry to the loader!", "Story"))
	assert.Equal(t, "fix/OPS-1-nightly-job-fails-on-null-ids", Name("OPS-1", "Nightly job fails on NULL ids", "Bug"))
	assert.Equal(t, "feat/X-1", Name("X-1", "   ", "Task"))
	got := Name("X-2", "a very long summary that keeps going and going well past the limit", "Task")
	assert.Equal(t, "feat/X-2-a-very-long-summary-that-keeps-going", got)
}
