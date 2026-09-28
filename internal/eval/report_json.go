package eval

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/FernasFragas/Nandocode/internal/tools/filewrite"
)

func WriteJSONReport(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return filewrite.AtomicWrite(path, data, 0o644)
}
