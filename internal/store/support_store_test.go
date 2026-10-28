package store

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bridge-core/internal/model"
)

func TestSupportStoreScopesUnreadMessagesAndConversationLifecycle(t *testing.T) {
	s, err := NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer s.Close()

	rootCustomer, err := s.CreateCustomer(model.CustomerAccountRequest{Username: "root-customer", Password: "password123"})
	if err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}
	enabled := true
	manager, err := s.CreateAreaManager(model.AreaManagerAccountRequest{Username: "area", Password: "password123", Enabled: &enabled})
	if err != nil {
		t.Fatalf("CreateAreaManager: %v", err)
	}
	areaCustomer, err := s.CreateCustomerForOwner(model.CustomerAccountRequest{Username: "area-customer", Password: "password123"}, model.AdminRoleAreaManager, manager.ID)
	if err != nil {
		t.Fatalf("CreateCustomerForOwner: %v", err)
	}

	if conversations, err := s.ListSupportConversations("", 0); err != nil || len(conversations) != 0 {
		t.Fatalf("empty conversation list = %#v, %v", conversations, err)
	}
	rootMessage, rootConversation, err := s.AddSupportMessage(rootCustomer.ID, model.SupportSenderCustomer, rootCustomer.ID, rootCustomer.Username, "root help")
	if err != nil {
		t.Fatalf("AddSupportMessage root: %v", err)
	}
	if rootMessage.Body != "root help" || rootConversation.UnreadCount != 1 {
		t.Fatalf("unexpected root support message: %#v %#v", rootMessage, rootConversation)
	}
	_, areaConversation, err := s.AddSupportMessage(areaCustomer.ID, model.SupportSenderCustomer, areaCustomer.ID, areaCustomer.Username, "area help")
	if err != nil {
		t.Fatalf("AddSupportMessage area: %v", err)
	}

	all, err := s.ListSupportConversations("", 0)
	if err != nil || len(all) != 2 {
		t.Fatalf("root conversation list = %#v, %v", all, err)
	}
	areaOnly, err := s.ListSupportConversations(model.AdminRoleAreaManager, manager.ID)
	if err != nil || len(areaOnly) != 1 || areaOnly[0].CustomerID != areaCustomer.ID {
		t.Fatalf("area conversation list = %#v, %v", areaOnly, err)
	}
	if count, err := s.CountUnreadSupportMessages("", 0); err != nil || count != 2 {
		t.Fatalf("root unread = %d, %v", count, err)
	}
	if count, err := s.CountUnreadSupportMessages(model.AdminRoleAreaManager, manager.ID); err != nil || count != 1 {
		t.Fatalf("area unread = %d, %v", count, err)
	}

	if err := s.MarkSupportConversationRead(rootConversation.ID, model.SupportSenderAdmin); err != nil {
		t.Fatalf("MarkSupportConversationRead admin: %v", err)
	}
	if count, err := s.CountUnreadSupportMessages("", 0); err != nil || count != 1 {
		t.Fatalf("root unread after read = %d, %v", count, err)
	}
	if _, _, err := s.AddSupportMessage(rootCustomer.ID, model.SupportSenderAdmin, 1, "Admin", "reply"); err != nil {
		t.Fatalf("AddSupportMessage reply: %v", err)
	}
	customerView, found, err := s.GetSupportConversationByCustomer(rootCustomer.ID, model.SupportSenderCustomer)
	if err != nil || !found || customerView.UnreadCount != 1 {
		t.Fatalf("customer unread conversation = %#v, found=%v err=%v", customerView, found, err)
	}
	if err := s.MarkSupportConversationRead(rootConversation.ID, model.SupportSenderCustomer); err != nil {
		t.Fatalf("MarkSupportConversationRead customer: %v", err)
	}
	customerView, _, _ = s.GetSupportConversationByCustomer(rootCustomer.ID, model.SupportSenderCustomer)
	if customerView.UnreadCount != 0 {
		t.Fatalf("expected customer unread to clear, got %#v", customerView)
	}

	if _, err := s.UpdateSupportConversationStatus(rootConversation.ID, model.SupportConversationClosed); err != nil {
		t.Fatalf("UpdateSupportConversationStatus: %v", err)
	}
	_, reopened, err := s.AddSupportMessage(rootCustomer.ID, model.SupportSenderCustomer, rootCustomer.ID, rootCustomer.Username, "one more question")
	if err != nil || reopened.Status != model.SupportConversationOpen {
		t.Fatalf("customer message should reopen conversation: %#v, %v", reopened, err)
	}

	if claimed, err := s.ClaimSupportNotification(areaConversation.ID, time.Minute); err != nil || !claimed {
		t.Fatalf("first notification claim = %v, %v", claimed, err)
	}
	if claimed, err := s.ClaimSupportNotification(areaConversation.ID, time.Minute); err != nil || claimed {
		t.Fatalf("duplicate notification claim = %v, %v", claimed, err)
	}
}

func TestSupportStoreValidatesMessageLengthAndRate(t *testing.T) {
	s, err := NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer s.Close()
	customer, err := s.CreateCustomer(model.CustomerAccountRequest{Username: "rate-user", Password: "password123"})
	if err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}
	if _, _, err := s.AddSupportMessage(customer.ID, model.SupportSenderCustomer, customer.ID, customer.Username, strings.Repeat("x", supportMessageMaxRunes+1)); err == nil {
		t.Fatal("expected oversized support message to fail")
	}
	for index := 0; index < supportMessageRateLimit; index++ {
		if _, _, err := s.AddSupportMessage(customer.ID, model.SupportSenderCustomer, customer.ID, customer.Username, "message"); err != nil {
			t.Fatalf("message %d: %v", index, err)
		}
	}
	if _, _, err := s.AddSupportMessage(customer.ID, model.SupportSenderCustomer, customer.ID, customer.Username, "too many"); !errors.Is(err, ErrSupportRateLimit) {
		t.Fatalf("expected rate limit error, got %v", err)
	}
}

func TestSupportStoreLinksGuestConversationIntoExistingCustomerThread(t *testing.T) {
	s, err := NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer s.Close()
	target, err := s.CreateCustomer(model.CustomerAccountRequest{Username: "linked-user", Password: "password123", DisplayName: "Linked User"})
	if err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}
	guest, err := s.GetOrCreateGuestCustomer("198.51.100.40", time.Hour, model.CustomerOwnerGuest, 0)
	if err != nil {
		t.Fatalf("GetOrCreateGuestCustomer: %v", err)
	}
	if _, _, err := s.AddSupportMessage(target.ID, model.SupportSenderCustomer, target.ID, target.DisplayName, "Existing account message"); err != nil {
		t.Fatalf("AddSupportMessage target: %v", err)
	}
	if _, _, err := s.AddSupportMessage(guest.ID, model.SupportSenderCustomer, guest.ID, guest.DisplayName, "Guest message"); err != nil {
		t.Fatalf("AddSupportMessage guest: %v", err)
	}
	linked, err := s.LinkGuestSupportConversation(guest.ID, target.ID)
	if err != nil || !linked {
		t.Fatalf("LinkGuestSupportConversation linked=%v err=%v", linked, err)
	}
	conversation, found, err := s.GetSupportConversationByCustomer(target.ID, model.SupportSenderAdmin)
	if err != nil || !found || conversation.IsGuest || conversation.LastMessagePreview != "Guest message" {
		t.Fatalf("unexpected linked conversation: %#v found=%v err=%v", conversation, found, err)
	}
	messages, err := s.ListSupportMessages(conversation.ID)
	if err != nil || len(messages) != 2 {
		t.Fatalf("linked messages = %#v err=%v", messages, err)
	}
	for _, message := range messages {
		if message.SenderRole == model.SupportSenderCustomer && message.SenderAccountID != target.ID {
			t.Fatalf("guest sender metadata was not reassigned: %#v", message)
		}
	}
	var guestCount int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM customer_accounts WHERE id = ?`, guest.ID).Scan(&guestCount); err != nil || guestCount != 0 {
		t.Fatalf("linked guest account should be removed: count=%d err=%v", guestCount, err)
	}
}
