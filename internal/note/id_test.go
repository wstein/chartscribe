// SPDX-License-Identifier: EUPL-1.2

package note

import "testing"

func TestValidID(t *testing.T) {
	valid := []string{"20261021120000", "20261021120000-001", "20240229235959", "00010101000000"}
	invalid := []string{
		"", "abc", "202610211200", "2026102112000", "202610211200000", "20261021120000-1", "20261021120000-0001",
		"20261021120000x", "20261321120000", "20261032120000", "20261021250000", "20261021126100",
		"20261021120060", "20230229120000", "00000101000000", "20261021120000\n", "２０２６１０２１１２００００",
	}
	for _, id := range valid {
		if !ValidID(id) {
			t.Errorf("ValidID(%q) = false", id)
		}
	}
	for _, id := range invalid {
		if ValidID(id) {
			t.Errorf("ValidID(%q) = true", id)
		}
	}
}
