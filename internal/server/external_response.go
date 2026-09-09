package server

import (
	"fmt"
	"io"
)

const maxExternalJSONResponseBytes = 4 << 20

func readExternalJSONResponse(body io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxExternalJSONResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxExternalJSONResponseBytes {
		return nil, fmt.Errorf("external JSON response exceeds %d bytes", maxExternalJSONResponseBytes)
	}
	return data, nil
}
