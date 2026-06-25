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
	// coversSymbol is only part of the contract when present. This keeps
	// existing specs (which don't set it) on their current hash; adding or
	// changing the binding is a contract change that invalidates the hash.
	if s.CoversSymbol != "" {
		payload["coversSymbol"] = s.CoversSymbol
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
