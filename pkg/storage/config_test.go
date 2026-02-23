package storage

import (
	"os"
	"testing"
)

func TestSetConfig(t *testing.T) {
	storage := setupTestStorage(t)
	defer func() {
		if err := storage.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	err := storage.SetConfig("test_key", "test_value")
	if err != nil {
		t.Fatalf("SetConfig failed: %v", err)
	}
}

func TestGetConfig(t *testing.T) {
	storage := setupTestStorage(t)
	defer func() {
		if err := storage.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	// Set config
	err := storage.SetConfig("test_get_key", "test_get_value")
	if err != nil {
		t.Fatalf("SetConfig failed: %v", err)
	}

	// Get config
	value, err := storage.GetConfig("test_get_key")
	if err != nil {
		t.Fatalf("GetConfig failed: %v", err)
	}
	if value != "test_get_value" {
		t.Errorf("GetConfig returned %q, want %q", value, "test_get_value")
	}
}

func TestGetConfigNotFound(t *testing.T) {
	storage := setupTestStorage(t)
	defer func() {
		if err := storage.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	value, err := storage.GetConfig("non_existent_key")
	if err != nil {
		t.Fatalf("GetConfig should not return error for non-existent key: %v", err)
	}
	if value != "" {
		t.Errorf("GetConfig should return empty string for non-existent key, got %q", value)
	}
}

func TestDeleteConfig(t *testing.T) {
	storage := setupTestStorage(t)
	defer func() {
		if err := storage.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	// Set config
	err := storage.SetConfig("test_delete_key", "test_value")
	if err != nil {
		t.Fatalf("SetConfig failed: %v", err)
	}

	// Delete config
	err = storage.DeleteConfig("test_delete_key")
	if err != nil {
		t.Fatalf("DeleteConfig failed: %v", err)
	}

	// Verify deletion
	value, err := storage.GetConfig("test_delete_key")
	if err != nil {
		t.Fatalf("GetConfig after delete failed: %v", err)
	}
	if value != "" {
		t.Errorf("GetConfig should return empty string after delete, got %q", value)
	}
}

func TestSetConfigOverwrite(t *testing.T) {
	storage := setupTestStorage(t)
	defer func() {
		if err := storage.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	// Set initial value
	err := storage.SetConfig("overwrite_key", "value1")
	if err != nil {
		t.Fatalf("SetConfig failed: %v", err)
	}

	// Overwrite
	err = storage.SetConfig("overwrite_key", "value2")
	if err != nil {
		t.Fatalf("SetConfig overwrite failed: %v", err)
	}

	// Verify new value
	value, err := storage.GetConfig("overwrite_key")
	if err != nil {
		t.Fatalf("GetConfig failed: %v", err)
	}
	if value != "value2" {
		t.Errorf("GetConfig returned %q, want %q", value, "value2")
	}
}

func TestConfigJSONValue(t *testing.T) {
	storage := setupTestStorage(t)
	defer func() {
		if err := storage.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	// Store JSON value
	jsonConfig := `{"bootstrap_timeout": 60000000000, "max_peers": 100}`
	err := storage.SetConfig("ipfs_config", jsonConfig)
	if err != nil {
		t.Fatalf("SetConfig failed: %v", err)
	}

	// Retrieve
	value, err := storage.GetConfig("ipfs_config")
	if err != nil {
		t.Fatalf("GetConfig failed: %v", err)
	}
	if value != jsonConfig {
		t.Errorf("GetConfig returned %q, want %q", value, jsonConfig)
	}
}

func TestStorageConcurrentConfig(t *testing.T) {
	storage := setupTestStorage(t)
	defer func() {
		if err := storage.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	// Concurrent config writes
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			key := "config_key_" + string(rune('0'+id))
			if err := storage.SetConfig(key, "value_"+string(rune('0'+id))); err != nil {
				t.Errorf("SetConfig failed: %v", err)
			}
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all configs were set
	for i := 0; i < 10; i++ {
		key := "config_key_" + string(rune('0'+i))
		value, err := storage.GetConfig(key)
		if err != nil {
			t.Fatalf("GetConfig failed: %v", err)
		}
		expected := "value_" + string(rune('0'+i))
		if value != expected {
			t.Errorf("Config %s = %q, want %q", key, value, expected)
		}
	}
}

func TestConfigPersistence(t *testing.T) {
	// Create temp directory
	dir, err := os.MkdirTemp("", "badger-config-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("RemoveAll failed: %v", err)
		}
	}()

	// Create storage and set config
	cfg := Config{Path: dir}
	storage, err := NewBadgerStorage(cfg)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	err = storage.SetConfig("persistent_key", "persistent_value")
	if err != nil {
		t.Fatalf("SetConfig failed: %v", err)
	}

	// Close storage
	if err := storage.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Reopen storage
	storage2, err := NewBadgerStorage(cfg)
	if err != nil {
		t.Fatalf("Failed to reopen storage: %v", err)
	}
	defer func() {
		if err := storage2.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	// Verify config persisted
	value, err := storage2.GetConfig("persistent_key")
	if err != nil {
		t.Fatalf("GetConfig after reopen failed: %v", err)
	}
	if value != "persistent_value" {
		t.Errorf("GetConfig returned %q, want %q", value, "persistent_value")
	}
}
