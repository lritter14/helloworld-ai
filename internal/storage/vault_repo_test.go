package storage

import (
	"context"
	"testing"
	"time"
)

func TestNewVaultRepo(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	repo := NewVaultRepo(db)
	if repo == nil {
		t.Fatal("NewVaultRepo() returned nil")
	}
}

func TestVaultRepo_GetOrCreateByName(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	repo := NewVaultRepo(db)

	tests := []struct {
		name     string
		vaultName string
		rootPath string
		wantErr  bool
		check    func(VaultRecord) bool
	}{
		{
			name:      "create new vault",
			vaultName: "test-vault",
			rootPath:  "/tmp/test",
			wantErr:   false,
			check: func(v VaultRecord) bool {
				return v.Name == "test-vault" && v.RootPath == "/tmp/test" && v.ID > 0
			},
		},
		{
			name:      "get existing vault",
			vaultName: "test-vault",
			rootPath:  "/tmp/test",
			wantErr:   false,
			check: func(v VaultRecord) bool {
				return v.Name == "test-vault"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vault, err := repo.GetOrCreateByName(context.Background(), tt.vaultName, tt.rootPath)

			if tt.wantErr {
				if err == nil {
					t.Errorf("GetOrCreateByName() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("GetOrCreateByName() unexpected error: %v", err)
				return
			}

			if tt.check != nil && !tt.check(vault) {
				t.Error("GetOrCreateByName() result validation failed")
			}
		})
	}
}

func TestVaultRepo_GetOrCreateByName_ReturnsSameVault(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	repo := NewVaultRepo(db)

	// Create vault
	vault1, err := repo.GetOrCreateByName(context.Background(), "same-vault", "/tmp/same")
	if err != nil {
		t.Fatalf("GetOrCreateByName() error = %v", err)
	}

	// Get same vault
	vault2, err := repo.GetOrCreateByName(context.Background(), "same-vault", "/tmp/different")
	if err != nil {
		t.Fatalf("GetOrCreateByName() error = %v", err)
	}

	// Should return same vault (by name)
	if vault1.ID != vault2.ID {
		t.Errorf("GetOrCreateByName() returned different IDs: %d vs %d", vault1.ID, vault2.ID)
	}

	if vault1.Name != vault2.Name {
		t.Errorf("GetOrCreateByName() returned different names: %s vs %s", vault1.Name, vault2.Name)
	}
}

func TestVaultRepo_ListAll(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	repo := NewVaultRepo(db)

	// Create multiple vaults
	vaults := []struct {
		name     string
		rootPath string
	}{
		{"z-vault", "/tmp/z"},
		{"a-vault", "/tmp/a"},
		{"m-vault", "/tmp/m"},
	}

	for _, v := range vaults {
		_, err := repo.GetOrCreateByName(context.Background(), v.name, v.rootPath)
		if err != nil {
			t.Fatalf("GetOrCreateByName() error = %v", err)
		}
	}

	// List all vaults
	allVaults, err := repo.ListAll(context.Background())
	if err != nil {
		t.Fatalf("ListAll() error = %v", err)
	}

	// Should be ordered by name
	if len(allVaults) < len(vaults) {
		t.Errorf("ListAll() returned %d vaults, want at least %d", len(allVaults), len(vaults))
	}

	// Check ordering (should be alphabetical)
	for i := 1; i < len(allVaults); i++ {
		if allVaults[i-1].Name > allVaults[i].Name {
			t.Errorf("ListAll() vaults not ordered: %s > %s", allVaults[i-1].Name, allVaults[i].Name)
		}
	}
}

func TestVaultRepo_ListAll_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	repo := NewVaultRepo(db)

	vaults, err := repo.ListAll(context.Background())
	if err != nil {
		t.Fatalf("ListAll() error = %v", err)
	}

	if len(vaults) != 0 {
		t.Errorf("ListAll() returned %d vaults, want 0", len(vaults))
	}
}

func TestVaultRecord_CreatedAt(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	repo := NewVaultRepo(db)

	vault, err := repo.GetOrCreateByName(context.Background(), "time-test", "/tmp/time")
	if err != nil {
		t.Fatalf("GetOrCreateByName() error = %v", err)
	}

	// Check that CreatedAt is set
	if vault.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}

	// Check that CreatedAt is recent (within last minute)
	if time.Since(vault.CreatedAt) > time.Minute {
		t.Error("CreatedAt should be recent")
	}
}

func TestVaultRepo_GetOrCreateByName_UniqueNames(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	repo := NewVaultRepo(db)

	// Create vault with unique name
	vault1, err := repo.GetOrCreateByName(context.Background(), "unique-1", "/tmp/1")
	if err != nil {
		t.Fatalf("GetOrCreateByName() error = %v", err)
	}

	// Try to create another vault with same name (should return existing)
	vault2, err := repo.GetOrCreateByName(context.Background(), "unique-1", "/tmp/2")
	if err != nil {
		t.Fatalf("GetOrCreateByName() error = %v", err)
	}

	// Should be same vault
	if vault1.ID != vault2.ID {
		t.Errorf("GetOrCreateByName() should return same vault for duplicate name")
	}

	// Create vault with different name
	vault3, err := repo.GetOrCreateByName(context.Background(), "unique-2", "/tmp/3")
	if err != nil {
		t.Fatalf("GetOrCreateByName() error = %v", err)
	}

	// Should be different vault
	if vault1.ID == vault3.ID {
		t.Errorf("GetOrCreateByName() should return different vault for different name")
	}
}

