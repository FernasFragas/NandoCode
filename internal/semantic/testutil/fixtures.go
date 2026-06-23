package testutil

import (
	"time"

	"github.com/FernasFragas/Nandocode/internal/semantic"
)

func FixtureManifest(root string, model string, dimensions int, workspaceID string) semantic.Manifest {
	now := time.Now().UTC().Truncate(time.Second)
	return semantic.Manifest{
		SchemaVersion: semantic.SchemaVersion,
		WorkspaceRoot: root,
		WorkspaceID:   workspaceID,
		Model:         model,
		Dimensions:    dimensions,
		CreatedAt:     now,
		UpdatedAt:     now,
		StorePreviews: true,
	}
}

func FixtureRecords() []semantic.Record {
	return []semantic.Record{
		{
			ID:          "rec-1",
			Kind:        semantic.RecordKindSymbol,
			Path:        "internal/server/auth.go",
			Name:        "validateBearerToken",
			StartLine:   10,
			EndLine:     30,
			ContentHash: semantic.HashText("auth-file"),
			TextHash:    semantic.HashText("symbol-1"),
			TextPreview: "validateBearerToken checks bearer auth",
			EmbedText:   "go symbol validateBearerToken auth validation",
		},
		{
			ID:          "rec-2",
			Kind:        semantic.RecordKindDocSection,
			Path:        "docs/AUTH.md",
			Name:        "Auth Overview",
			StartLine:   1,
			EndLine:     20,
			ContentHash: semantic.HashText("auth-doc"),
			TextHash:    semantic.HashText("doc-1"),
			TextPreview: "Authentication architecture details",
			EmbedText:   "documentation section authentication architecture",
		},
	}
}
