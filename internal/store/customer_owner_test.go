package store

import (
	"reflect"
	"testing"

	"bridge-core/internal/model"
)

func TestCustomerOwnerUpdateRollsBackWithFrontProxyFailure(t *testing.T) {
	s, customer := customerSessionFixture(t)
	manager, err := s.CreateAreaManager(model.AreaManagerAccountRequest{Username: "receiving-area"})
	if err != nil {
		t.Fatal(err)
	}
	proxy, err := s.CreateFrontProxyNode(model.FrontProxyNodeRequest{Name: "Front", ShareURL: "socks5://proxy.example:1080"})
	if err != nil {
		t.Fatal(err)
	}
	pair := mustCustomerPair(t, s, customer.ID)
	// Force a database failure after the account UPDATE and session revocation,
	// so both must roll back together with the ownership and grant changes.
	if _, err := s.db.Exec(`CREATE TRIGGER reject_customer_front_proxy BEFORE INSERT ON front_proxy_grants
		WHEN NEW.grantee_type = 'customer' BEGIN SELECT RAISE(ABORT, 'test grant failure'); END`); err != nil {
		t.Fatal(err)
	}
	_, err = s.UpdateCustomer(customer.ID, model.CustomerAccountRequest{
		OwnerAreaManagerID: &manager.ID, Password: "changed-password", DisplayName: "must-not-save", FrontProxyNodeIDs: []int64{proxy.ID},
	})
	if err == nil {
		t.Fatal("expected front proxy write to fail")
	}
	after, _, err := s.GetCustomer(customer.ID)
	if err != nil || !reflect.DeepEqual(customer, after) {
		t.Fatalf("ownership and account must roll back: before=%#v after=%#v err=%v", customer, after, err)
	}
	assertCustomerAccess(t, s, pair.AccessToken, true)
	if _, ok, err := s.AuthenticateCustomer(customer.Username, "password123"); err != nil || !ok {
		t.Fatalf("failed update changed password: ok=%v err=%v", ok, err)
	}
}
