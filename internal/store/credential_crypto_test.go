package store

import (
	"path/filepath"
	"sync"
	"testing"
)

func TestLoadOrCreateCredentialCipherIsSafeForConcurrentStarts(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "keys", "credential.key")
	const workers = 12

	ciphers := make([]*CredentialCipher, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := range ciphers {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			cipher, err := LoadOrCreateCredentialCipher(keyPath)
			if err == nil {
				ciphers[index] = cipher
			}
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("LoadOrCreateCredentialCipher: %v", err)
		}
	}

	ciphertext, err := ciphers[0].EncryptString("panel-secret")
	if err != nil {
		t.Fatalf("EncryptString: %v", err)
	}
	for index, cipher := range ciphers {
		plaintext, err := cipher.DecryptString(ciphertext)
		if err != nil || plaintext != "panel-secret" {
			t.Fatalf("cipher %d cannot decrypt shared key output: plaintext=%q err=%v", index, plaintext, err)
		}
	}
}
