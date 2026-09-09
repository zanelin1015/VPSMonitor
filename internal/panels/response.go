package panels

import (
	"fmt"
	"io"
)

const maxPanelResponseBytes = 32 << 20

func readPanelResponse(body io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxPanelResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxPanelResponseBytes {
		return nil, fmt.Errorf("panel response exceeds %d bytes", maxPanelResponseBytes)
	}
	return data, nil
}
