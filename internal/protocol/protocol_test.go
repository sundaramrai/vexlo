package protocol

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

func TestDecodeRejectsOversizedFrameBeforeAllocation(t *testing.T) {
	var input bytes.Buffer
	if err := binary.Write(&input, binary.BigEndian, uint32(MaxMessageSize+1)); err != nil {
		t.Fatal(err)
	}
	_, err := Decode(bufio.NewReader(&input), nil)
	if err == nil || !strings.Contains(err.Error(), "invalid tunnel frame size") {
		t.Fatalf("expected frame limit error, got %v", err)
	}
}
