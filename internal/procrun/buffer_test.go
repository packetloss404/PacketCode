package procrun

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBoundedBufferKeepsFirstBytesAndReportsTruncation(t *testing.T) {
	b := NewBoundedBuffer(5)
	n, err := b.Write([]byte("abc"))
	assert.NoError(t, err)
	assert.Equal(t, 3, n)
	n, err = b.Write([]byte("defgh"))
	assert.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "abcde", b.String())
	assert.True(t, b.Truncated())
}

func TestBoundedBufferDiscardsWhenLimitIsZero(t *testing.T) {
	b := NewBoundedBuffer(0)
	n, err := b.Write([]byte(strings.Repeat("x", 10)))
	assert.NoError(t, err)
	assert.Equal(t, 10, n)
	assert.Empty(t, b.String())
	assert.True(t, b.Truncated())
}
