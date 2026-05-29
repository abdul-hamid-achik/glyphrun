package spec

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

func ComputeContractHash(s Spec) (string, error) {
	payload := map[string]any{
		"intent":   s.Intent,
		"outcomes": s.Outcomes,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
