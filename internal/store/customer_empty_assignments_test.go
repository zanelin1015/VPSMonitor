package store

import (
	"encoding/json"
	"testing"

	"bridge-core/internal/model"
)

func TestListCustomersSerializesEmptyAssignmentsAsArray(t *testing.T) {
	s, customer := customerSessionFixture(t)
	for name, list := range map[string]func() ([]model.CustomerAdminView, error){
		"all": s.ListCustomers,
		"owner": func() ([]model.CustomerAdminView, error) {
			return s.ListCustomersForOwner(customer.OwnerType, customer.OwnerID)
		},
	} {
		t.Run(name, func(t *testing.T) {
			customers, err := list()
			if err != nil || len(customers) != 1 || customers[0].ID != customer.ID {
				t.Fatalf("unexpected customer list: %#v, %v", customers, err)
			}
			encoded, err := json.Marshal(customers)
			if err != nil {
				t.Fatal(err)
			}
			var response []struct {
				Assignments json.RawMessage `json:"assignments"`
			}
			if err := json.Unmarshal(encoded, &response); err != nil {
				t.Fatal(err)
			}
			if string(response[0].Assignments) != "[]" {
				t.Fatalf("unassigned users must return an array, got %s", encoded)
			}
		})
	}
}
