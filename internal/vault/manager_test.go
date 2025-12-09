package vault

import (
	"context"
	"testing"

	"helloworld-ai/internal/storage"
	"helloworld-ai/internal/storage/mocks"
	"go.uber.org/mock/gomock"
)

func TestNewManager(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

		mockVaultRepo := mocks.NewMockVaultStore(ctrl)

	// Setup mocks for personal and work vaults
	mockVaultRepo.EXPECT().
		GetOrCreateByName(gomock.Any(), "personal", "/tmp/personal").
		Return(storage.VaultRecord{ID: 1, Name: "personal", RootPath: "/tmp/personal"}, nil)

	mockVaultRepo.EXPECT().
		GetOrCreateByName(gomock.Any(), "work", "/tmp/work").
		Return(storage.VaultRecord{ID: 2, Name: "work", RootPath: "/tmp/work"}, nil)

	manager, err := NewManager(context.Background(), mockVaultRepo, "/tmp/personal", "/tmp/work")
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	if manager == nil {
		t.Fatal("NewManager() returned nil")
	}
}

func TestNewManager_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

		mockVaultRepo := mocks.NewMockVaultStore(ctrl)

	mockVaultRepo.EXPECT().
		GetOrCreateByName(gomock.Any(), "personal", "/tmp/personal").
		Return(storage.VaultRecord{}, storage.ErrNotFound)

	manager, err := NewManager(context.Background(), mockVaultRepo, "/tmp/personal", "/tmp/work")
	if err == nil {
		t.Error("NewManager() expected error, got nil")
	}
	if manager != nil {
		t.Error("NewManager() should return nil on error")
	}
}

func TestManager_VaultByName(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

		mockVaultRepo := mocks.NewMockVaultStore(ctrl)

	personalVault := storage.VaultRecord{ID: 1, Name: "personal", RootPath: "/tmp/personal"}
	workVault := storage.VaultRecord{ID: 2, Name: "work", RootPath: "/tmp/work"}

	mockVaultRepo.EXPECT().
		GetOrCreateByName(gomock.Any(), "personal", "/tmp/personal").
		Return(personalVault, nil)

	mockVaultRepo.EXPECT().
		GetOrCreateByName(gomock.Any(), "work", "/tmp/work").
		Return(workVault, nil)

	manager, err := NewManager(context.Background(), mockVaultRepo, "/tmp/personal", "/tmp/work")
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	tests := []struct {
		name    string
		vaultName string
		wantErr bool
		check   func(storage.VaultRecord) bool
	}{
		{
			name:      "existing vault",
			vaultName: "personal",
			wantErr:   false,
			check: func(v storage.VaultRecord) bool {
				return v.Name == "personal" && v.ID == 1
			},
		},
		{
			name:      "non-existent vault",
			vaultName: "nonexistent",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vault, err := manager.VaultByName(tt.vaultName)

			if tt.wantErr {
				if err == nil {
					t.Errorf("VaultByName() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("VaultByName() unexpected error: %v", err)
				return
			}

			if tt.check != nil && !tt.check(vault) {
				t.Error("VaultByName() result validation failed")
			}
		})
	}
}

func TestManager_AbsPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

		mockVaultRepo := mocks.NewMockVaultStore(ctrl)

	personalVault := storage.VaultRecord{ID: 1, Name: "personal", RootPath: "/tmp/personal"}
	workVault := storage.VaultRecord{ID: 2, Name: "work", RootPath: "/tmp/work"}

	mockVaultRepo.EXPECT().
		GetOrCreateByName(gomock.Any(), "personal", "/tmp/personal").
		Return(personalVault, nil)

	mockVaultRepo.EXPECT().
		GetOrCreateByName(gomock.Any(), "work", "/tmp/work").
		Return(workVault, nil)

	manager, err := NewManager(context.Background(), mockVaultRepo, "/tmp/personal", "/tmp/work")
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	tests := []struct {
		name    string
		vaultID int
		relPath string
		want    string
	}{
		{
			name:    "personal vault",
			vaultID: 1,
			relPath: "notes/test.md",
			want:    "/tmp/personal/notes/test.md",
		},
		{
			name:    "work vault",
			vaultID: 2,
			relPath: "projects/doc.md",
			want:    "/tmp/work/projects/doc.md",
		},
		{
			name:    "root level file",
			vaultID: 1,
			relPath: "root.md",
			want:    "/tmp/personal/root.md",
		},
		{
			name:    "non-existent vault",
			vaultID: 999,
			relPath: "test.md",
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := manager.AbsPath(tt.vaultID, tt.relPath)
			if got != tt.want {
				t.Errorf("AbsPath(%d, %q) = %q, want %q", tt.vaultID, tt.relPath, got, tt.want)
			}
		})
	}
}

